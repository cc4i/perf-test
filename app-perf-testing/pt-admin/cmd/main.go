package main

import (
	"context"
	"os"

	"com.google.gtools/pt-admin/internal"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {

	opts := zap.Options{
		Development: false,
		DestWriter:  os.Stdout,
	}
	l := zap.New(zap.UseFlagOptions(&opts))

	internal.Router(context.Background(), l).Run("0.0.0.0:9080")
}
