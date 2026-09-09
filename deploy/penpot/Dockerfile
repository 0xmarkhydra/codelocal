# CodeLocal keeps Penpot upstream intact. This thin image pins the tested
# upstream release and applies only the deployment bridge required by the
# codelocal.cloud dashboard embedding boundary.
FROM penpotapp/frontend:2.17.0

USER root
COPY nginx-security-headers.conf /etc/nginx/nginx-security-headers.conf
COPY nginx-mcp-locations.conf.template /tmp/nginx-mcp-locations.conf.template
COPY codelocal-mcp-bootstrap.js /var/www/app/codelocal-mcp-bootstrap.js

# Penpot 2.17.0 contains the upstream ui.css fix. Keep the documented upstream
# compatibility fallback so a rebuilt image cannot regress to an HTML 404 when
# a registry mirror serves an older/broken frontend layer. The CodeLocal bridge
# is injected only into this thin deployment image; upstream Penpot stays intact.
RUN if [ ! -e /var/www/app/css/ui.css ] && [ -e /var/www/app/css/main.css ]; then \
      ln -s main.css /var/www/app/css/ui.css; \
    fi && \
    [ "$(grep -o 'hidden:d' /var/www/app/plugins/mcp/plugin.js | wc -l)" -eq 1 ] && \
    sed -i 's/hidden:d/hidden:!1/' /var/www/app/plugins/mcp/plugin.js && \
    ! grep -q 'hidden:d' /var/www/app/plugins/mcp/plugin.js && \
    cp /var/www/app/plugins/mcp/plugin.js /var/www/app/plugins/mcp/plugin-codelocal-v1.js && \
    cp -a /var/www/app/plugins/mcp /var/www/app/plugins/mcp-codelocal-v1 && \
    [ "$(grep -o '"plugin.js"' /var/www/app/js/shared.js | wc -l)" -eq 1 ] && \
    sed -i 's/"plugin.js"/"plugin-codelocal-v1.js"/' /var/www/app/js/shared.js && \
    [ "$(grep -o '"plugins/mcp/"' /var/www/app/js/shared.js | wc -l)" -eq 1 ] && \
    sed -i 's#"plugins/mcp/"#"plugins/mcp-codelocal-v1/"#' /var/www/app/js/shared.js && \
    [ "$(grep -o 'shared.js?version=2.17.0-1784702688' /var/www/app/index.html | wc -l)" -eq 1 ] && \
    sed -i 's/shared.js?version=2.17.0-1784702688/shared.js?version=2.17.0-1784702688-codelocal2/' /var/www/app/index.html && \
    grep -q '</body>' /var/www/app/index.html && \
    sed -i 's#</body>#<script src="/codelocal-mcp-bootstrap.js?v=2" defer></script></body>#' /var/www/app/index.html

USER penpot:penpot
