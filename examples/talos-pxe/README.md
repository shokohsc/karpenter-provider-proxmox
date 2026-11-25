# Talos PXE Boot on Proxmox

This example provisions Talos Linux worker nodes by booting their Proxmox VMs over PXE,
without passing a Talos machine configuration through the provider.

## How it works

When `bootMethod: pxe` is set on the `ProxmoxNodeClass`:

- The provider skips cloud-init ISO generation and detachment.
- No kubelet bootstrap token is created.
- Network configuration is not regenerated and no address is allocated from the IP pool.
  The VM must get its address from DHCP, and the provider leaves the template's network
  configuration untouched.
- The VM boots from the network. The PXE/TFTP server supplies the Talos kernel, initrd and
  kernel arguments, including the URL of the machine configuration
  (`talos.config=https://config.example.com/config.yaml`).
- Talos fetches its machine configuration from the remote server, installs, and joins the
  cluster. Karpenter marks the node as ready once it registers.

The provider does not run a TFTP server. Use an existing PXE environment (for example
dnsmasq, iPXE or PXELINUX) and point it at the Talos kernel and initrd.

## Prerequisites

1. A PXE/TFTP server serving the Talos kernel and initrd, with kernel arguments that
   include `talos.platform=metal` and `talos.config=<url>`.
2. A remote HTTP(S) server serving the Talos machine configuration.
3. A Proxmox VM template named `talos-pxe`, configured to boot from the network and
   tagged with `talos` and `pxe`. The template can contain an empty disk for the Talos
   installation.

An example PXELINUX entry:

```text
DEFAULT talos
LABEL talos
  KERNEL talos/vmlinuz
  INITRD talos/initramfs.xz
  APPEND initrd=talos/initramfs.xz talos.platform=metal talos.config=https://config.example.com/config.yaml
```

You can match the VM's MAC address in the PXE server configuration, or serve the same
kernel arguments to every VM. The Talos machine configuration URL can encode the node
identity if each node needs a different configuration.

## Proxmox template

Create a VM template with:

- a disk for the Talos installation (the node class resizes it to at least `bootDevice.size`),
- a network device on the bridge the PXE server is reachable from,
- network boot before the disk in the boot order,
- no cloud-init drive required.

Then convert the VM to a template and tag it:

```shell
qm set <vmid> --name talos-pxe --boot order=net0
qm template <vmid>
pvesh set /nodes/<node>/qemu/<vmid>/config --tags talos,pxe
```

## Deploy the example

```shell
kubectl apply -f examples/talos-pxe/unmanaged-template.yaml
kubectl apply -f examples/talos-pxe/nodeclass.yaml
```

The NodePool then provisions Talos nodes over PXE. Changing `bootMethod` on an existing
node class triggers drift, because it changes how nodes boot and join the cluster.
