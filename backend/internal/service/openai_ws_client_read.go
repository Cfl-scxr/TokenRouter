package service

import (
	"context"
	"errors"
	"time"

	coderws "github.com/coder/websocket"
)

type openAIWSClientReadResult struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

// ReadOpenAIWSClientMessage 在控制事件发送关闭帧期间保留唯一读协程，
// 随后强制关闭底层连接并等待读协程退出。
func ReadOpenAIWSClientMessage(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
) (coderws.MessageType, []byte, error) {
	if conn == nil {
		return 0, nil, errors.New("openai websocket client connection is nil")
	}
	if controlCtx == nil {
		controlCtx = context.Background()
	}

	readDone := make(chan openAIWSClientReadResult, 1)
	go func() {
		messageType, payload, err := conn.Read(context.Background())
		readDone <- openAIWSClientReadResult{messageType: messageType, payload: payload, err: err}
	}()

	var timeoutCh <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutCh = timer.C
		defer timer.Stop()
	}

	closeAndJoin := func(status coderws.StatusCode, reason string, cause error) (coderws.MessageType, []byte, error) {
		_ = conn.Close(status, reason)
		_ = conn.CloseNow()
		<-readDone
		return 0, nil, NewOpenAIWSClientCloseError(status, reason, cause)
	}

	select {
	case result := <-readDone:
		return result.messageType, result.payload, result.err
	case <-timeoutCh:
		return closeAndJoin(timeoutStatus, timeoutReason, context.DeadlineExceeded)
	case <-controlCtx.Done():
		cause := context.Cause(controlCtx)
		if errors.Is(cause, ErrOpenAIWSIngressLeaseLost) {
			return closeAndJoin(
				coderws.StatusTryAgainLater,
				"websocket ingress capacity lease lost; please reconnect",
				cause,
			)
		}
		return closeAndJoin(coderws.StatusGoingAway, "websocket request canceled", cause)
	}
}
