#!/bin/bash

# Define variables
LOCAL_DIR="./public/"
REMOTE_USER="worm"
REMOTE_HOST="cloud"
REMOTE_DIR="/var/www/efans.gay"

# Run rsync
rsync -avz --delete \
    --exclude='.DS_Store' \
    -e ssh \
    "${LOCAL_DIR}" \
    "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}"

# Check the exit status
if [ $? -eq 0 ]; then
    echo "Sync completed successfully."
else
    echo "Sync failed. Please check your connection and try again."
fi
