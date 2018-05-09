# d2p-migrator

d2p-migrator is a system tool for ops to migrate containers from docker world
to pouchcontainer world.

## Scenaris

PouchContainer has a lot of advanced features towards Docker. Under some
circumstance, when upgrading dockerd to pouchd, the legacy containers cannot be
removed. These containers must be running in place. As a result, it is an
essential functionality to migrate legacy containers to those which are managed
by pouchcontainer.

## Migrate Steps

We assume that dockerd is running with controlling containerd 0.2.3 in
environment. Docker has not supported containerd 1.0.0+ yet, so image
management is not in the scope of docker. While PouchContainer takes
advantanges of containerd 1.0.0+ to manage containers and images. Migration
containers from docker's management to pouchd must take both image and
container into consideration. Here are the steps to accomplish a migration job.

### Step 1 - Install containerd 1.0.3 independently

### Step 2 - Pull Images via ctr

### Step 3 - Prepare snapshotters with containerd's API

### Step 4 - Setting QuotaID to UpperDir/MergedDir

### Step 5 - Stop all containers and dockerd

### Step 6 - Move legacy content from old_upperdir/* to new-upperdir/

### Step 7 - Convert old container's metadata in dockerd to new's in pouchd

### Step 8 - Stop running containerd 1.0.3

### Step 9 - Install PouchContainer with rpm including containerd 1.0.3

### Step 10 - Start all containers in pouchd

## To be continued