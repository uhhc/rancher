package cluster

import (
	"context"
	"fmt"

	"github.com/rancher/rke/docker"
	"github.com/rancher/rke/hosts"
	"github.com/rancher/rke/log"
	"github.com/rancher/rke/services"
	"github.com/rancher/rke/util"
)

func (c *Cluster) SnapshotEtcd(ctx context.Context, snapshotName string) error {
	backupImage := c.getBackupImage()
	if !util.IsRancherBackupSupported(c.SystemImages.Alpine) {
		log.Warnf(ctx, "Auto local backup sync is not supported in `%s`. Using `%s` instead.", c.SystemImages.Alpine, backupImage)
	}
	for _, host := range c.EtcdHosts {
		if err := services.RunEtcdSnapshotSave(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotName, true, c.Services.Etcd); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) PrepareBackup(ctx context.Context, snapshotPath string) error {
	// local backup case
	var backupServer *hosts.Host
	backupImage := c.getBackupImage()
	if !util.IsRancherBackupSupported(c.SystemImages.Alpine) {
		log.Warnf(ctx, "Auto local backup sync is not supported in `%s`. Using `%s` instead.", c.SystemImages.Alpine, backupImage)
	}
	if c.Services.Etcd.BackupConfig == nil || // legacy rke local backup
		(c.Services.Etcd.BackupConfig != nil && c.Services.Etcd.BackupConfig.S3BackupConfig == nil) { // rancher local backup, no s3
		// stop etcd on all etcd nodes, we need this because we start the backup server on the same port
		for _, host := range c.EtcdHosts {
			if err := docker.StopContainer(ctx, host.DClient, host.Address, services.EtcdContainerName); err != nil {
				log.Warnf(ctx, "failed to stop etcd container on host [%s]: %v", host.Address, err)
			}
			if backupServer == nil { // start the download server, only one node should have it!
				if err := services.StartBackupServer(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath); err != nil {
					log.Warnf(ctx, "failed to start backup server on host [%s]: %v", host.Address, err)
					continue
				}
				backupServer = host
			}
		}
		// start downloading the snapshot
		for _, host := range c.EtcdHosts {
			if backupServer != nil && host.Address == backupServer.Address { // we skip the backup server if it's there
				continue
			}
			if err := services.DownloadEtcdSnapshotFromBackupServer(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath, backupServer); err != nil {
				return err
			}
		}
		// all good, let's remove the backup server container
		if err := docker.DoRemoveContainer(ctx, backupServer.DClient, services.EtcdServeBackupContainerName, backupServer.Address); err != nil {
			return err
		}
	}

	// s3 backup case
	if c.Services.Etcd.BackupConfig != nil && c.Services.Etcd.BackupConfig.S3BackupConfig != nil {
		for _, host := range c.EtcdHosts {
			if err := services.DownloadEtcdSnapshotFromS3(ctx, host, c.PrivateRegistriesMap, backupImage, snapshotPath, c.Services.Etcd); err != nil {
				return err
			}
		}
	}

	// this applies to all cases!
	if isEqual := c.etcdSnapshotChecksum(ctx, snapshotPath); !isEqual {
		return fmt.Errorf("etcd snapshots are not consistent")
	}
	return nil
}

func (c *Cluster) RestoreEtcdSnapshot(ctx context.Context, snapshotPath string) error {
	// Start restore process on all etcd hosts
	initCluster := services.GetEtcdInitialCluster(c.EtcdHosts)
	for _, host := range c.EtcdHosts {
		if err := services.RestoreEtcdSnapshot(ctx, host, c.PrivateRegistriesMap, c.SystemImages.Etcd, snapshotPath, initCluster); err != nil {
			return fmt.Errorf("[etcd] Failed to restore etcd snapshot: %v", err)
		}
	}
	return nil
}

func (c *Cluster) etcdSnapshotChecksum(ctx context.Context, snapshotPath string) bool {
	log.Infof(ctx, "[etcd] Checking if all snapshots are identical")
	etcdChecksums := []string{}
	backupImage := c.getBackupImage()

	for _, etcdHost := range c.EtcdHosts {
		checksum, err := services.GetEtcdSnapshotChecksum(ctx, etcdHost, c.PrivateRegistriesMap, backupImage, snapshotPath)
		if err != nil {
			return false
		}
		etcdChecksums = append(etcdChecksums, checksum)
		log.Infof(ctx, "[etcd] Checksum of etcd snapshot on host [%s] is [%s]", etcdHost.Address, checksum)
	}
	hostChecksum := etcdChecksums[0]
	for _, checksum := range etcdChecksums {
		if checksum != hostChecksum {
			return false
		}
	}
	return true
}

func (c *Cluster) getBackupImage() string {
	if util.IsRancherBackupSupported(c.SystemImages.Alpine) {
		return c.SystemImages.Alpine
	}
	return util.GetDefaultRKETools()
}
