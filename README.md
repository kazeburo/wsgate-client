# wsgate-client - a websocket to tcp proxy/bridge client server

Please see https://github.com/kazeburo/wsgate-server for usage.

```
[client]
|
| TCP
|
[wsgate-client] (https://github.com/kazeburo/wsgate-client)
|
| websocket (wss)
|
[reverse proxy] if required
|
| websocket (ws)
|
[wsgate-server] (https://github.com/kazeburo/wsgate-server)
|
| TCP
|
[server]
```

## Usage

```
Usage:
  wsgate-client [OPTIONS]

Application Options:
      --map=              listen port and upstream url mapping file
      --connect-timeout=  timeout of connection to upstream (default: 60s)
      --shutdown-timeout= timeout to wait for all connections to be closed. (default: 1h)
  -v, --version           Show version
      --headers=          Header key and value added to upsteam
      --private-key=      private key for signing JWT auth header
      --private-key-user= user id which is used as Subject in JWT payload (default: private-key-user)
      --iap-credential=   GCP service account json file for using wsgate -server behind IAP enabled Cloud Load Balancer
      --iap-client-id=    IAP's OAuth2 Client ID

Help Options:
  -h, --help              Show this help message
```
