# Build the React app and serve it via nginx (proxying /api to the API service).
# Build context is the repo root: docker build -f deploy/web.Dockerfile .
FROM node:24-alpine AS build
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM nginx:alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
