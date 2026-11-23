# Build locally first: mise run //modules/inventory:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/inventory /inventory
EXPOSE 8081 8082
ENTRYPOINT ["/inventory"]
CMD ["service"]
