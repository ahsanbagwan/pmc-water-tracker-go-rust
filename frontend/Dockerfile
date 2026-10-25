# Stage 1: build the static assets
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
# VITE_API_BASE gets baked in at BUILD time, same mechanic covered earlier for Vercel --
# passed here as a Docker build arg since there's no Vercel dashboard doing this for you now.
ARG VITE_API_BASE
ENV VITE_API_BASE=${VITE_API_BASE}
RUN npm run build

# Stage 2: serve the built files
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
