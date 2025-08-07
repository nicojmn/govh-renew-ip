# govh-renew-ip

govh-renew-ip is a simple script to renew the IP address of an ovh domain using the OVH API. It is primarely designed to be run in
a docker container but you can use it on any flavour you like.

## Prerequisites

- A domain name registered with OVH
- OVH API credentials ([Find it here](https://api.ovh.com/createToken/))

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

- From source code :

```bash
git clone https://github.com/nicojmn/govh-renew-ip.git
cd govh-renew-ip
go mod tidy
go run main.go # for development
# or
go build -o renew-ip main.go # for production
```

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
    image: nicojmn/govh-renew-ip:latest
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

## Contributing

If you want to contribute to this project, feel free to open an issue or a pull request. Any contributions are welcome !

## Misc

### Building the image

If you want to build the image yourself, you can use the following command:

```bash
docker build -t your_image_name .
```

### How does it work ?

The script performs the following operations :

1. Get the current public IP address using an external service (ipify).
2. Get A records from the provided domain using the OVH API.
3. Check if the current public IP address found in at least one of the A records.
4. If the current public IP address is not found in any of the A records, update the A records with the new public IP address using the OVH API.
5. Repeat the process every 2 minutes.
6. If the script is stopped, it will stop the loop and exit.

### Why not use a dynamic DNS service ?

- Dynamic DNS services are great, but they are not always free or easy to set up.
- ISPs may have weird rules or behaviors
- This script is lightweight, 



## TODO

- [x] Get records ID and target IP
- [x] Detect if the IP address is already in the A records
- [x] Add record when the IP address is not found
- [x] Docker setup
- [x] Github Actions setup
- [x] Update record instead of adding it
- [x] Add support for AAAA records
- [x] Handle subdomains
