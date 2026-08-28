FROM node:22.12.0 AS builder
WORKDIR /app

COPY pnpm-lock.yaml pnpm-workspace.yaml package.json tsconfig.json ./
COPY packages ./packages
COPY shell ./shell
COPY plugins ./plugins

RUN npm install -g pnpm@8.15.0 && pnpm install --frozen-lockfile

RUN pnpm build:all

FROM nginx:alpine
COPY --from=builder /app/shell/dist /usr/share/nginx/html/shell
COPY --from=builder /app/plugins/dashboard/dist /usr/share/nginx/html/modules/dashboard
COPY --from=builder /app/plugins/application/dist /usr/share/nginx/html/modules/application
COPY --from=builder /app/plugins/container/dist /usr/share/nginx/html/modules/container
COPY --from=builder /app/plugins/resource/dist /usr/share/nginx/html/modules/resource
COPY --from=builder /app/plugins/service/dist /usr/share/nginx/html/modules/service
COPY --from=builder /app/plugins/ai/dist /usr/share/nginx/html/modules/ai
COPY --from=builder /app/plugins/system/dist /usr/share/nginx/html/modules/system
COPY shell/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
