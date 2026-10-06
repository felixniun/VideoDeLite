#!/bin/bash
# VideoDelite admin UI + stats deployment (run as root on the server).
# Usage: VD_ADMIN_KEY=<your-admin-key> bash deploy-admin.sh
# The key is taken from the environment - never hardcode it here.
set -e
: "${VD_ADMIN_KEY:?set VD_ADMIN_KEY to the admin key first}"
cd /home/debian/videodelite

mv /tmp/vd_adminui   server/adminui.html 2>/dev/null || true
mv /tmp/vd_handlers  server/handlers.go 2>/dev/null || true
mv /tmp/vd_store     server/store.go 2>/dev/null || true
mv /tmp/vd_storeimpl server/store_impl.go 2>/dev/null || true

# admin key change (value comes from VD_ADMIN_KEY env)
sed -i "s/\"adminKey\": \"[^\"]*\"/\"adminKey\": \"${VD_ADMIN_KEY}\"/" config.json 2>/dev/null || true
sed -i "s/VIDEODELITE_ADMIN_KEY: .*/VIDEODELITE_ADMIN_KEY: \"${VD_ADMIN_KEY}\"/" docker-compose.yml 2>/dev/null || true

docker compose up -d --build server 2>&1 | tail -2
sleep 3
echo "--- health:"
curl -s -m 5 http://127.0.0.1:8800/api/v1/version
echo
curl -s -o /dev/null -w "admin page: %{http_code}\n" http://127.0.0.1:8800/admin
curl -s -H "X-Admin-Key: ${VD_ADMIN_KEY}" http://127.0.0.1:8800/api/v1/admin/stats | head -c 300
echo
