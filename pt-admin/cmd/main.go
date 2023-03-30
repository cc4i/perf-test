package main

import (
	"context"
	"os"

	"com.google.gtools/pt-admin/internal"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {

	opts := zap.Options{
		Development: false,
		DestWriter:  os.Stdout,
	}
	l := zap.New(zap.UseFlagOptions(&opts))
	log.SetLogger(l)
	l.Info("starting pt-admin")
	internal.Router(context.Background()).Run("0.0.0.0:9080")

}
