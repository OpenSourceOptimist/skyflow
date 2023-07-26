# skyflow
The Scalabe Go Nostr Relay

Early days but have quite a component test coverage and basic tests
with Amethyst. Instance running live at relay.nostry.eu (not ready to be relied upon!)

Goals:

* Simple
* Correct
* Horizontally scalable
* Fast

In that order. I want to support as many NIPs as possible but only if
I can assure the above goals.

## NIPs support

- [x] [NIP01](https://github.com/nostr-protocol/nips/blob/master/01.md)


## Setup https relay

```yaml
version: "2.1"

services:
  skyflow:
    image: ghcr.io/opensourceoptimist/skyflow:v0.1.7
    container_name: skyflow
    environment:
      - MONGODB_URI=mongodb://mongo:27017
    expose:
      - 80
    restart: unless-stopped
  mongo:
    image: docker.io/mongo:6.0.5
    expose:
      - 27017
    restart: unless-stopped
    volumes:
      - mongo_db_data:/data/db
  caddy:
    build:
      context: ./caddy
    ports:
      - 443:443
    volumes:
     - ./caddy_data:/data
     - ./caddy_config:/config
    restart: unless-stopped

volumes:
  mongo_db_data:
```

Create a folder named `caddy` and put the Dockerfile and Caddyfile in
the folder.

Dockerfile:
```Dockerfile
FROM caddy:2.6.4
COPY Caddyfile /etc/caddy/Caddyfile
```

Caddyfile:
```
relay.example.com:443 {
    reverse_proxy skyflow:80

    tls me@example.com

    log {
        output file /var/log/caddy/nostr.log {
            roll_size 1gb
            roll_keep 1
            roll_keep_for 720h
        }
    }
}
```

You will have to register a domain and point the DNS to your IP and
make sure you have port 443 open in your firewall/router.

You can now connect to your relay in any nostr app with
`wss://relay.example.com`.
