# Talos PXE Boot on Proxmox

This example provisions Talos Linux worker nodes via PXE network boot,
without cloud-init metadata.

## Prerequisites

1. A PXE/TFTP server serving the Talos kernel and initrd
2. A Proxmox VM template configured for PXE boot (network-first boot order, no cloud-init)
3. The template tagged with `talos` and `pxe`

## How It Works

When `bootMethod: pxe` is set on the ProxmoxNodeClass:

- The provider skips cloud-init ISO generation entirely
- No bootstrap token is created
- No IPAM allocation is performed (VM gets IP via DHCP from PXE server)
- The VM boots from network, loads Talos via PXE
- Talos fetches its machine config from the Talos API or a configured URL
- After Talos installs and joins the cluster, Karpenter marks the node as ready

## Setup

1. Create a PXE-ready Proxmox template:
   - Create a VM with a network device (virtio, bridged)
   - Set boot order: `net0` first
   - Optionally add a small disk for Talos installation
   - Convert to template
   - Tag with `talos` and `pxe`

2. Apply the resources:
   ```bash
   kubectl apply -f unmanaged-template.yaml
   kubectl apply -f nodeclass.yaml
   ```

3. The NodePool will now provision Talos nodes via PXE boot.
