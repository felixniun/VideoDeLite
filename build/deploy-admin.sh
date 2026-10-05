#!/bin/bash
# VideoDelite admin UI + stats deployment (run as root on the server)
set -e
cd /home/debian/videodelite

mv /tmp/vd_adminui   server/adminui.html
mv /tmp/vd_handlers  server/handlers.go
mv /tmp/vd_store     server/store.go
mv /tmp/vd_storeimpl server/store_impl.go

# admin key change (approved by owner)
sed -i 's/ChangeMe_StrongAdminKey/Aa147258/' docker-compose.yml
grep -n 'ADMIN_KEY' docker-compose.yml

docker compose up -d --build server 2>&1 | tail -2
sleep 3
echo "--- health:"
curl -s -m 5 http://127.0.0.1:8800/api/v1/version
echo
curl -s -o /dev/null -w "admin page: %{http_code}\n" http://127.0.0.1:8800/admin
curl -s -H "X-Admin-Key: Aa147258" http://127.0.0.1:8800/api/v1/admin/stats | head -c 300
echo
