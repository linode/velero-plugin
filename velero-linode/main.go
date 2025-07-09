package main

import (
	"github.com/sirupsen/logrus"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		RegisterVolumeSnapshotter("linode.com/velero", newVolumeSnapshotter).
		Serve()
}

func newVolumeSnapshotter(logger logrus.FieldLogger) (interface{}, error) {
	return &VolumeSnapshotter{FieldLogger: logger}, nil
}
