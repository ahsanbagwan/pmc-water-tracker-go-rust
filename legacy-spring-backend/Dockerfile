# --- Build stage ---
FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline -B
COPY src ./src
RUN mvn clean package -DskipTests

# --- Runtime stage ---
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /app/target/*.jar app.jar

# Runs as a real (non-root) user rather than the container default root -- a container escape
# from a root-owned process has far more potential blast radius on the host than one running
# as an unprivileged user. Kubernetes-specific value: this also satisfies the
# securityContext.runAsNonRoot: true policy set in the Deployment manifest below, which many
# cluster admission policies enforce regardless of what you'd prefer.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080
# -Xms/-Xmx set explicitly rather than left to the JVM's container-aware auto-detection
# (which guesses ~25% of the container's memory limit by default) -- tied directly to this
# project's actual K8s resource requests (512Mi) / limits (1Gi) from k8s/02-backend.yaml.
# -Xms == -Xmx avoids heap-resize pauses during warmup; leaves ~250Mi headroom under the 1Gi
# limit for thread stacks, metaspace, and off-heap buffers, which don't come out of the heap
# but do count against the container's memory limit.
ENTRYPOINT ["java", "-Xms512m", "-Xmx750m", "-jar", "app.jar"]
