#!/bin/bash
set -e

# Build for linux
GOOS=linux GOARCH=amd64 go build -o efansgay

# Stop service
ssh worm@cloud 'sudo systemctl stop efansgay'

# Copy files to server
scp efansgay worm@cloud:/home/worm/efansgay/
scp .env worm@cloud:/home/worm/efansgay/
scp -r public worm@cloud:/home/worm/efansgay/

# Start service
ssh worm@cloud 'sudo systemctl start efansgay'

echo "Deployment complete!" 