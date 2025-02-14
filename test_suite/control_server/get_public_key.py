# Reads and prints hexadecimal part of "ControlKey" field from control.json

import json

# Read file to retrieve current config
with open("control.json", "r") as f:
    config = json.load(f)