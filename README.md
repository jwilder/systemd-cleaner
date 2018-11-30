# systemd-cleaner

Service to cleanup leaked transient file mounts from k8s pods

This is a workaround service for a systemd bug that is fixed in systemd 237, but is not available in older Ubuntu 16.04 LTS releases.

It specifically fixes the issue reported in [#57345](https://github.com/kubernetes/kubernetes/issues/57345)

# Installation

Install the deb from the release and enable the service on the host.  It will run periodically and cleanup leaked file mounts units.