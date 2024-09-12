# This script can be used to automatically update control.json to include a relay server
# Usage: python configure_json.py <RELAY SERVER PUBLIC KEY> <RELAY SERVER IP> <RELAY SERVER PORT>
import json
import sys

# Read file to retrieve current config
with open("control.json") as f:
    config = json.load(f)


# Update config 
relay_pub_key, relay_ip, relay_port = sys.argv[1:4]
relay_dict = {
    "ID": 0,
    "Key": f"pubkey:{relay_pub_key}",
    "IPs": [
        relay_ip
    ],
    "IsInsecure": True,
    "HTTPPort": int(relay_port)
}
config["Relays"] = [relay_dict]

# Overwrite file
with open("control.json", "w") as f:
    f.write(json.dumps(config, indent=4))
