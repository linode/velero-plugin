package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	"github.com/pkg/errors"
	"github.com/satori/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// Linode Snapshot names have a 32 character limit.
	linodeVolumeLabelLen = 32
)

// VolumeSnapshotter handles talking to Linode API & logging
type VolumeSnapshotter struct {
	client *linodego.Client
	config map[string]string
	logrus.FieldLogger
}

// TokenSource is a Linode API token
type TokenSource struct {
	AccessToken string
}

// Init the plugin
//
// Init prepares the VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config.
func (b *VolumeSnapshotter) Init(config map[string]string) error {
	b.Infof("BlockStore.Init called")
	b.config = config

	tokenSource := &TokenSource{
		AccessToken: os.Getenv("LINODE_TOKEN"),
	}

	oauthClient := oauth2.NewClient(context.Background(), tokenSource)
	client := linodego.NewClient(oauthClient)
	b.client = &client
	b.client.SetDebug(true)

	return nil
}

// Token returns an oauth2 token from an API key
func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}

	return token, nil
}

// CreateVolumeFromSnapshot makes a volume from a stored backup snapshot
//
// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot,
// and with the specified type and IOPS (if using provisioned IOPS).
func (b *VolumeSnapshotter) CreateVolumeFromSnapshot(snapshotID string, volumeType string, volumeAZ string, iops *int64) (volumeID string, err error) {
	b.Infof("CreateVolumeFromSnapshot called with snapshotID %s", snapshotID)

	ctx := context.TODO()
	sid, err := strconv.Atoi(snapshotID)
	if err != nil {
		b.Errorf("snapshotID is not numeric: %v", err)
		return "", err
	}

	// TODO(displague) should we bother fetching this?
	_, err = b.client.GetVolume(ctx, sid)
	if err != nil {
		b.Errorf("GetVolume returned error: %v", err)
	}

	label := ("restore-" + snapshotID + "-" + uuid.NewV4().String())[:linodeVolumeLabelLen]

	newVolume, err := b.client.CloneVolume(ctx, sid, label)
	if err != nil {
		b.Errorf("CloneVolume returned error: %v", err)
	}

	return strconv.Itoa(newVolume.ID), nil
}

// parseVolumeID splits and parses Velero VolumeID into a Linode VolumeID
// and Label.  Expected volumeID format is "123-pvc1a2b3c".
func parseVolumeID(volumeID string) (int, string, error) {
	parts := strings.SplitN(volumeID, "-", 2)

	vid, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("Could not parse VolumeID: %v", err)
	}
	return vid, parts[1], nil
}

// GetVolumeInfo fetches volume information from the Linode API
//
// GetVolumeInfo returns the type and IOPS (if using provisioned IOPS) for
// the specified volume in the given availability zone.
func (b *VolumeSnapshotter) GetVolumeInfo(volumeID string, volumeAZ string) (string, *int64, error) {
	b.Infof("GetVolumeInfo called with volumeID %s", volumeID)

	ctx := context.TODO()

	vid, _, err := parseVolumeID(volumeID)
	if err != nil {
		b.Error(err)
		return "", nil, err
	}

	// TODO(displague) should we bother fetching this?
	_, err = b.client.GetVolume(ctx, vid)
	if err != nil {
		b.Errorf("GetVolume returned error: %v", err)
	}

	// TODO(displague) raw? ext4?
	return "auto", nil, nil
}

// IsVolumeReady just returns true
//
// TODO(displague) deprecated?
func (b *VolumeSnapshotter) IsVolumeReady(volumeID string, volumeAZ string) (ready bool, err error) {
	return true, nil
}

// CreateSnapshot makes a clone of a persistent volume using the Linode API
//
// CreateSnapshot creates a snapshot of the specified volume, and applies the
// provided set of tags to the snapshot.
func (b *VolumeSnapshotter) CreateSnapshot(volumeID string, volumeAZ string, tags map[string]string) (string, error) {
	b.Infof("CreateSnapshot called with volumeID %s", volumeID)

	// TODO(displague) VolumeID is 25+ chars. most of the uuid is trimmed.
	// Use tags for added context?
	snapshotName := ("vol" + volumeID + "-" + uuid.NewV4().String())[:linodeVolumeLabelLen]

	vid, _, err := parseVolumeID(volumeID)
	if err != nil {
		b.Error(err)
		return "", err
	}

	ctx := context.TODO()

	b.Infof("CreateSnapshot trying to clone volume")
	newVolume, err := b.client.CloneVolume(ctx, vid, snapshotName)
	if err != nil {
		b.Errorf("CloneVolume returned error: %v", err)
	}
	ltags := []string{}
	for k, v := range tags {
		ltags = append(ltags, k+":"+v)
	}

	_, err = b.client.UpdateVolume(ctx, newVolume.ID, linodego.VolumeUpdateOptions{
		Tags: &ltags,
	})
	if err != nil {
		b.Errorf("UpdateVolume returned error: %v", err)
	}

	return strconv.Itoa(newVolume.ID), nil
}

// DeleteSnapshot deletes the specified volume (cloned or not)
func (b *VolumeSnapshotter) DeleteSnapshot(volumeID string) error {
	b.Infof("DeleteSnapshot called with snapshotID %v", volumeID)

	ctx := context.TODO()

	vid, _, err := parseVolumeID(volumeID)
	if err != nil {
		b.Error(err)
		return err
	}

	err = b.client.DeleteVolume(ctx, vid)
	if err != nil {
		b.Errorf("DeleteVolume returned error: %v", err)
	}

	return err
}

// GetVolumeID returns the cloud provider specific identifier for the
// PersistentVolume.
func (b *VolumeSnapshotter) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	b.Infof("GetVolumeID called with unstructuredPV %v", unstructuredPV)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", errors.WithStack(err)
	}
	if pv.Spec.CSI == nil {
		return "", fmt.Errorf("unable to retrieve CSI Spec from pv %+v", pv)
	}
	if pv.Spec.CSI.VolumeHandle == "" {
		return "", fmt.Errorf("unable to retrieve Volume handle from pv %+v", pv)
	}
	return pv.Spec.CSI.VolumeHandle, nil
}

// SetVolumeID sets the cloud provider specific identifier for the
// PersistentVolume.
func (b *VolumeSnapshotter) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	b.Infof("SetVolumeID called with unstructuredPV %v and volumeID %s", unstructuredPV, volumeID)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, errors.WithStack(err)
	}

	if pv.Spec.CSI == nil {
		return nil, fmt.Errorf("spec.CSI not found from pv %+v", pv)
	}

	pv.Spec.CSI.VolumeHandle = volumeID

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: res}, nil
}
