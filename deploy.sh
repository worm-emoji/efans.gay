#!/bin/bash
set -e

# Build for linux
GOOS=linux GOARCH=amd64 go build -o efansgay

# Stop service
ssh worm@cloud 'sudo systemctl stop efansgay'

# Copy files to server using rsync
rsync -avz --progress --ignore-existing \
    efansgay .env public/ \
    worm@cloud:/home/worm/efansgay/

# Start service
ssh worm@cloud 'sudo systemctl start efansgay'

echo "Deployment complete!" 