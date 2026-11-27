# Build locally first: mise run //modules/recipes:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/recipes /recipes
EXPOSE 8081 8082
ENTRYPOINT ["/recipes"]
CMD ["service"]
