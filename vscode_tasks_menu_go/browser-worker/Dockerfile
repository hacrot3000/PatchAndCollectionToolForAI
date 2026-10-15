# Optional isolated Chromium service. Not part of TaskDeck's slim Go image.
# Keep this container rootless and enforce a host/container EGRESS FIREWALL.
FROM mcr.microsoft.com/playwright:v1.64.0-noble
WORKDIR /opt/taskdeck-browser
COPY package.json ./
RUN npm install --omit=dev --ignore-scripts --no-audit --no-fund
COPY worker.mjs semantic_extract.mjs ./
USER pwuser
ENTRYPOINT ["node","/opt/taskdeck-browser/worker.mjs"]
