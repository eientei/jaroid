This dsicord bot downloads videos from [nicovideo.jp](https://www.nicovideo.jp/), 
provides color roles, role reactions and proxied pins

USAGE
---
Set up a discord application with bot associated https://discord.com/developers/docs/intro ; note the discord bot token.

Run as `./jaroid -c config.yml`

Bot implementation requires a redis server to store persistent configuration and queues

Config 
---

Example structure

```yaml
private:
  token: "<discord bot token>"
  prefix: "!"
  redis:
    address: "127.0.0.1:6379"
    password: ""
    db: 0
  nicovideo:
    directory: "/home/somewhere/public/nicovideo"
    public: "http://example.com/nicovideo"
    period: "24h"
    auth:
      username: ""
      password: ""
```

`!nico.download` will place files in `nicovideo.directory` and post a link using `nicovideo.public` as base, hence directory
should be served by some HTTP server.

Example nginx configuration:
```
server {
  server_name example.com;
  listen 80;

  root /home/somewhere/public/nicovideo;
  
  location / {
    autoindex on;
  }
}
```

Setting up a feed
---

```
!nico.feed <name> <period> <channelID> <search filter>
```

e.g. !nico.feed cookie 1h 1234567890 クッキー☆
