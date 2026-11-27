# Build locally first: mise run //modules/bff:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/bff /bff
EXPOSE 8081 8082
ENTRYPOINT ["/bff"]
CMD ["service"]
