package main

import (
	"context"
	"runtime"

	stub "github.com/banzaicloud/hpa-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	"github.com/sirupsen/logrus"
	"os"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()
	namespace := os.Getenv("OPERATOR_NAMESPACE")
	sdk.Watch("apps/v1", "Deployment", namespace, 0)
	sdk.Watch("apps/v1", "StatefulSet", namespace, 0)
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
