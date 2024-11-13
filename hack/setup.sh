#!/bin/bash

LATESTv="$(curl -s https://api.github.com/repos/ctfer-io/chall-manager/releases/latest | jq -r '.tag_name')"
LATEST=${LATESTv#v}

# Download binaries
wget -qO- "https://github.com/ctfer-io/chall-manager/releases/download/${LATESTv}/chall-manager_${LATEST}_linux_amd64.tar.gz" | tar -xz --exclude='README.md' --exclude='LICENSE'
wget -qO- "https://github.com/ctfer-io/chall-manager/releases/download/${LATESTv}/chall-manager-janitor_${LATEST}_linux_amd64.tar.gz" | tar -xz  --exclude='README.md' --exclude='LICENSE'

# Verify integrity
wget -qO- "https://github.com/ctfer-io/chall-manager/releases/download/${LATESTv}/multiple.intoto.jsonl"

slsa-verifier verify-artifact "chall-manager"  \
  --provenance-path "multiple.intoto.jsonl"  \
  --source-uri "github.com/ctfer-io/chall-manager" \
  --source-tag "$LATESTv"

slsa-verifier verify-artifact "chall-manager-janitor"  \
  --provenance-path "multiple.intoto.jsonl"  \
  --source-uri "github.com/ctfer-io/chall-manager" \
  --source-tag "$LATESTv"

rm multiple.intoto.jsonl

# Install in PATH
install chall-manager chall-manager-janitor /usr/local/bin/

# Create user and group
sudo useradd --system --no-create-home --shell /usr/sbin/nologin chall-manager
sudo groupadd chall-manager
sudo usermod -aG chall-manager chall-manager

sudo mkdir -p /var/lib/chall-manager
sudo chown -R chall-manager:chall-manager /var/lib/chall-manager
sudo chmod -R 750 /var/lib/chall-manager

# Create chall-manager service
cat <<EOF | sudo tee /etc/systemd/system/chall-manager.service
[Unit]
Description=Chall-Manager by CTFer.io
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/chall-manager
Restart=on-failure
User=chall-manager
Group=chall-manager

[Install]
WantedBy=multi-user.target
EOF

# Create chall-manager-janitor service and timer
cat <<EOF | sudo tee /etc/systemd/system/chall-manager-janitor.service 
[Unit]
Description=Chall-Manager Janitor by CTFer.io
After=chall-manager.service
User=chall-manager
Group=chall-manager

[Service]
ExecStart=/usr/local/bin/chall-manager-janitor
EOF

cat <<EOF | sudo tee /etc/systemd/system/chall-manager-janitor.timer

[Unit]
Description=Runs Chall-Manager Janitor every minute

[Timer]
OnCalendar=*:0/1
Persistent=true

[Install]
WantedBy=timers.target
EOF

# Start services and timers
sudo systemctl daemon-reload
sudo systemctl enable chall-manager
sudo systemctl start chall-manager
sudo systemctl enable chall-manager-janitor.timer
sudo systemctl start chall-manager-janitor.timer
