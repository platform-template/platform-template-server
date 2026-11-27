# Build locally first: mise run //modules/shopping:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/shopping /shopping
EXPOSE 8081 8082
ENTRYPOINT ["/shopping"]
CMD ["service"]
