FROM node:22-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:alpine
COPY --from=build /app/dist /usr/share/nginx/html
# The unprivileged nginx image listens on 8080 so run unprivileged
EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
