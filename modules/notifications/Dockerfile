# Build locally first: mise run //modules/notifications:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/notifications /notifications
EXPOSE 8081 8082
ENTRYPOINT ["/notifications"]
CMD ["service"]
