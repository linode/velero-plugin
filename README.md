## Getting Started

This README explains how to install and configure the Linode Block Storage provider plugin for [Velero](https://velero.io). The plugin is designed to create filesystem  snapshots of Block Storage backed [PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) that are used in a Kubernetes cluster running on Linode.

- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Credentials setup](#credentials-setup)
  - [Velero installation](#velero-installation)
  - [Snapshot configuration](#snapshot-configuration)
  - [Backup and restore example](#backup-and-restore-example)
  - [Build the plugin](#build-the-plugin)

### Prerequisites

- A Kubernetes cluster running on Linode. It can be a managed cluster or self-hosted
- Linode account and resources
  - [API personal access token](https://www.linode.com/docs/platform/api/getting-started-with-the-linode-api/)
  - [Object Storage access keys](https://www.linode.com/docs/platform/object-storage/how-to-use-object-storage/)
  - Object Storage bucket
  - Object Storage bucket region
- [Velero](https://velero.io/docs/v1.2.0/basic-install/) v1.20 or newer & prerequisites

### Credentials setup

1. To use this plugin with Velero to create persistent volume snapshots, you
   will need a [Linode API
   token](https://www.linode.com/docs/platform/api/getting-started-with-the-linode-api/).
   Create one before proceeding with the rest of these steps.

1. For the object storage Velero component, generate an [Object Storage access
   key and secret
   key](https://www.linode.com/docs/platform/object-storage/how-to-use-object-storage/)

### Velero installation

1. Complete the Prerequisites and Credentials setup steps mentioned above.
1. Clone this repository. `cd` into the `examples` directory and edit the `cloud-credentials` file. The file will look like this:

    ```ini
    [default]
    aws_access_key_id=<AWS_ACCESS_KEY_ID>
    aws_secret_access_key=<AWS_SECRET_ACCESS_KEY>
    ```

   Edit the `<AWS_ACCESS_KEY_ID>` and `<AWS_SECRET_ACCESS_KEY>` placeholders to use your Linode Object Storage keys. Be sure to remove the `<` and `>` characters.
1. Still in the `examples` directory, edit the `01-velero-secret.patch.yaml` file. It should look like this:

    ```yaml
    ---
    apiVersion: v1
    kind: Secret
    stringData:
    linode_token: <LINODE_API_TOKEN>
    type: Opaque
    ```

   - Change the entire `<LINODE_API_TOKEN>` portion to use your Linode personal API token. The line should look something like `linode_token: 18a0d730c0e0....`
1. Now you're ready to install velero, configure the snapshot storage location,
   and work with backups. Ensure that you edit each of the following settings to
   match your Spaces configuration befor running the `velero install` command:

   - `--bucket velero-backups` - Ensure you change the `velero-backups` value to match the name of your Space.
   - `--backup-location-config s3Url=https://us-east-1.linodeobjects.com,region=us-east-1` - Change the URL and region to match your Object Storage settings. Specifically, edit the `us-east-1` portion in both to match the region where your Space is hosted. Use one of `us-east-1`, `eu-central-1` depending on your region.
1. Now run the install command:

   ```sh
   velero install \
    --provider velero.io/aws \
    --bucket velero-backups \
    --plugins velero/velero-plugin-for-aws:v1.0.1,displague/velero-plugin-linode:v0.0.1 \
    --backup-location-config s3Url=https://us-east-1.linodeobjects.com,region=us-east-1 \
    --use-volume-snapshots=false \
    --secret-file=./cloud-credentials
   ```

### Snapshot configuration

1. Enable the `displague/velero-plugin-linode:v0.0.1` snapshot provider. This command will configure Velero to use the plugin for persistent volume snapshots.

   ```sh
   velero snapshot-location create default --provider linode.com/velero
   ```

1. Patch the `cloud-credentials` Kubernetes Secret object that the `velero
   install` command installed in the cluster. This command will add your Linode
   API token to the `cloud-credentials` object so that this plugin can use the
   Linode API:

   ```sh
   kubectl patch secret cloud-credentials -p "$(cat 01-velero-secret.patch.yaml)" --namespace velero
   ```

1. Patch the `velero` Kubernetes Deployment to expose your API token to the
   Velero pod(s). Velero needs this change in order to authenticate to the
   Linode API when manipulating snapshots:

   ```sh
   kubectl patch deployment velero -p "$(cat 02-velero-deployment.patch.yaml)" --namespace velero
   ```

### Backup and restore example

1. Install the Nginx `examples/nginx-example.yaml` Deployment into your cluster. The example uses a persistent volume for Nginx logs. It also creates a NodeBalancer with a public IP address:

    ```sh
    kubectl apply -f examples/nginx-example.yaml
    ```

1. Ensure that your Nginx Deployment is running and there is a Service with an `EXTERNAL-IP` (`kubectl get service --namespace nginx-example`). Browse the IP a few times to write some log entries to the persistent volume. Then create a backup with Velero:

   ```sh
   velero backup create nginx-backup --selector app=nginx --snapshot-volumes=true
   velero backup describe nginx-backup --details
   ```

1. The various backup files will be in your Spaces bucket. A snapshot of the persistent volume will be listed in the Linode control panel under the [*Volumes* tab](https://cloud.linode.com/volumes). Now you can simulate a disaster by deleting the `nginx-example` namespace.

   ```sh
   kubectl delete namespace nginx-example
   ```

1. Once the delete finishes, restore the `nginx-backup` backup:

   ```sh
   velero restore create --from-backup nginx-backup
   ```

1. Check the restored PersistentVolume, Deployment, and Service are back using
   `kubectl`:

   ```sh
   kubectl get persistentvolume --namespace nginx-example
   kubectl get service --namespace nginx-example
   kubectl get deployment --namespace nginx-example
   ```

### Build the plugin

```sh
make clean
make container IMAGE=displague/velero-plugin-linode:dev
```
