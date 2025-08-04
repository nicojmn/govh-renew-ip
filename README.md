# govh-renew-ip

govh-renew-ip is a simple script to renew the IP address of an ovh domain using the OVH API. It is primarely designed to be run in
a docker container but you can use it on any flavour you like.

## How does it work ?

The script performs the following operations :

1. Get the current public IP address using an external service (ipify).
2. Get A records from the provided domain using the OVH API.
3. Check if the current public IP address found in at least one of the A records.
4. If the current public IP address is not found in any of the A records, update the A records with the new public IP address using the OVH API.
5. Repeat the process every 2 minutes.
6. If the script is stopped, it will stop the loop and exit.

## Why do I need this ?

Well, if you have never heard of *a infamous Belgian ISP* (i.e. your ISP hasn't a working dynDNS), you probably don't need this. But if you are like me and might be impacted by an unannounced change of your public IP address, you might want to use this script to update your domain name with the new IP.

## Prerequisites

- Docker / Compose / Swarm / Kubernetes / ...
- OVH account
- OVH API credentials ([Find it here](https://api.ovh.com/createToken/))
- A domain name registered with OVH

## Configuration

- Add the following environment variables to your docker-compose.yml file or .env file

```yaml
OVH_ENDPOINT=your_ovh_endpoint
OVH_APPKEY=your_ovh_application_key
OVH_APP_SECRET=your_ovh_application_secret
OVH_CONSUMER_KEY=your_ovh_consumer_key
DOMAIN= your_domain_name
TIME_INTERVAL=value_in_seconds
```

## Usage

- Docker CLI :

```bash
docker run -e OVH_ENDPOINT=$OVH_ENDPOINT \
-e OVH_APP_KEY=$OVH_APP_KEY \
-e OVH_APP_SECRET=$OVH_APP_SECRET \
-e OVH_CONSUMER_KEY=$OVH_CONSUMER_KEY \
-e TIME_INTERVAL=$TIME_INTERVAL \
-e DOMAIN=$DOMAIN nicojmn/govh-renew-ip:latest
```

- Docker Compose :

```yaml
services:
  renew-ip:
    image: nicojmn/govh-renew-ip:dev
    container_name: renew-ip
    environment:
      - OVH_ENDPOINT=${OVH_ENDPOINT}
      - OVH_APP_KEY=${OVH_APP_KEY}
      - OVH_APP_SECRET=${OVH_APP_SECRET}
      - OVH_CONSUMER_KEY=${OVH_CONSUMER_KEY}
      - DOMAIN=${DOMAIN}
      - TIME_INTERVAL=${TIME_INTERVAL}
    restart: unless-stopped
```

## Building the image

If you want to build the image yourself, you can use the following command:

```bash
docker build -t your_image_name .
```

## TODO

- [x] Get records ID and target IP
- [x] Detect if the IP address is already in the A records
- [x] Add record when the IP address is not found
- [x] Docker setup
- [x] Github Actions setup
- [] Update record instead of adding it
- [x] Add support for AAAA records
