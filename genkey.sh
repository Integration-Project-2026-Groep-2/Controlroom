 #!/bin/sh

KEY=$(openssl rand -hex 16)

if [ -f .env ]; then
    echo "xpack.encryptedSavedObjects.encryptionKey=$KEY" >> .env
else
    echo "xpack.encryptedSavedObjects.encryptionKey=$KEY" > .key
fi
