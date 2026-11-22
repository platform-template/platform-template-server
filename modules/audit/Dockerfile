# Build locally first: mise run //modules/audit:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/audit /audit
EXPOSE 8081 8082
ENTRYPOINT ["/audit"]
CMD ["service"]
