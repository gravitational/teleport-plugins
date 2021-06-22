# fluentd forwarder example key file
storage = "./storage"
timeout = "10s"
batch = 20
namespace = "default"

[fluentd]
ca = "{{index .CaPaths 0}}"
cert = "{{index .ClientPaths 0}}"
key = "{{index .ClientPaths 1}}"
url = "https://localhost:8888/test.log"

[teleport]
addr = "{{.Addr}}"
identity = "identity"
