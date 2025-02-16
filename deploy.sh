#!/bin/bash
set -e

# Build for linux
GOOS=linux GOARCH=amd64 go build -o efansgay

# Copy binary to temp location first
scp efansgay worm@cloud:/home/worm/efansgay/efansgay.new
ssh worm@cloud 'chmod +x /home/worm/efansgay/efansgay.new'

# Copy other files
scp .env worm@cloud:/home/worm/efansgay/
scp -r public worm@cloud:/home/worm/efansgay/

# Move new binary into place and restart service
ssh worm@cloud 'mv /home/worm/efansgay/efansgay.new /home/worm/efansgay/efansgay && sudo systemctl restart efansgay'
echo "Deployment complete!" 