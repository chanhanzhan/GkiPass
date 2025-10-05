package main

import (
	"net/http"
	_ "net/http/pprof"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// StartPProfServer 启动pprof服务器
func StartPProfServer(addr string) {
	logger.Info("pprof性能分析已启动",
		zap.String("addr", "http://localhost"+addr))

	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.Error("pprof服务器错误", zap.Error(err))
	}
}








