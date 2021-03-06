# Teleport Application Access Node.js JWT Example App

Teleport can be used to secure access to internal dashboards and applications. This
sample application provides automatic access using JSON Web Tokens (JWTs). This
application restricts access to a specific Teleport Proxy. Once logged in, it'll
show the Teleport roles available to the application.

Prerequisites
- A Teleport Cluster running 5.1.0 or greater.
    - Teleport Cloud [Signup](https://goteleport.com/get-started/)
    - Teleport self-hosted [quickstart](https://goteleport.com/teleport/docs/quickstart/)
- Node.js local environment with Teleport running locally.

### Configuring the App:
- Update .env `TELEPORT_PROXY` with the public address and port of your Teleport Cluster

### Run it locally:

1. Clone this repo, install and run the app.
```bash
npm install
TELEPORT_PROXY=example.teleport.sh:443 node ./app.js
```

2. [Install Teleport](https://goteleport.com/teleport/docs/installation/) locally, in this setup Teleport will dial back to Teleport Cloud.

Get a App Token
```bash
tctl tokens add --type=app
```

Start Teleport:
```bash
# Update --auth-server to your Teleport Cloud account or on-prem proxy address
# Update token and ca-pin with your values from the previous Get a App Token step
sudo teleport start --roles=app --auth-server=example.teleport.sh:443 \
    --app-name="jwt-quickstart" \
    --app-uri="http://localhost:8080" \
    --token=c28a84b64f50ssss28959969f8576c26b \
     --ca-pin=sha256:cda9df84cc4e1f0a9f6b41011e0e
```
