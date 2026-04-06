package nodeagent

import "log/slog"

type managedErrCloser interface {
	Close() error
}

type managedCloser interface {
	Close()
}

func closeManagedComponents(logger *slog.Logger, logMessage string, components ...any) {
	for _, component := range components {
		switch closer := component.(type) {
		case nil:
			continue
		case managedErrCloser:
			if err := closer.Close(); err != nil {
				logger.Error(logMessage, "error", err)
			}
		case managedCloser:
			closer.Close()
		}
	}
}
