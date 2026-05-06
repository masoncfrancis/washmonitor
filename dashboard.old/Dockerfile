# --- Build Stage ---
FROM node:22-alpine AS builder

WORKDIR /app

ARG NODE_ENV=production

COPY package*.json ./
RUN npm i

COPY . .

ARG NEXT_TELEMETRY_DISABLED=1

EXPOSE 3000

RUN npm run build

CMD ["npm", "start"]

## --- Nginx Stage ---
#FROM nginx:alpine
#
## Copy exported static site from build stage
#COPY --from=builder /app/out /usr/share/nginx/html
#
## Copy custom nginx config
#COPY nginx.conf /etc/nginx/nginx.conf
#
#EXPOSE 8002
#
#CMD ["nginx", "-g", "daemon off;"]