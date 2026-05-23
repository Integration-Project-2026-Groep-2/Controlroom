#!/bin/sh

KEY=$(openssl rand -hex 16)

if [ -f .env ]; then
    if grep -q "^KIBANA_ENCRYPTION_KEY=" .env; then
        # probeer die lijn weg te doen met kibana en dan gewoon vervangen snapje colle dingen
        sed -i "s/^KIBANA_ENCRYPTION_KEY=.*/&$KEY/" .env
    else
        echo "KIBANA_ENCRYPTION_KEY=$KEY" >> .env
    fi
else
    echo "KIBANA_ENCRYPTION_KEY=$KEY" > .env
fi
