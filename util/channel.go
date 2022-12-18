package util

import "github.com/goware/logger"

// NOTE: please see https://github.com/goware/channel for an improved version of the below method.
//
// converts a blocking unbuffered send channel into a non-blocking unbounded buffered one
// inspired by https://medium.com/capital-one-tech/building-an-unbounded-channel-in-go-789e175cd2cd
func MakeUnboundedChan[V any](sendCh chan<- V, log logger.Logger, bufferLimitWarning int) chan<- V {
	ch := make(chan V)

	go func() {
		var buffer []V

		for {
			if len(buffer) == 0 {
				if message, ok := <-ch; ok {
					buffer = append(buffer, message)
					if len(buffer) > bufferLimitWarning {
						log.Warnf("channel buffer holds %v > %v messages", len(buffer), bufferLimitWarning)
					}
				} else {
					close(sendCh)
					break
				}
			} else {
				select {
				case sendCh <- buffer[0]:
					buffer = buffer[1:]

				case message, ok := <-ch:
					if ok {
						buffer = append(buffer, message)
						if len(buffer) > bufferLimitWarning {
							log.Warnf("channel buffer holds %v > %v messages", len(buffer), bufferLimitWarning)
						}
					}
				}
			}
		}
	}()

	return ch
}
