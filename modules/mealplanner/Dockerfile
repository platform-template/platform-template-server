# Build locally first: mise run //modules/mealplanner:build
FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/mealplanner /mealplanner
EXPOSE 8081 8082
ENTRYPOINT ["/mealplanner"]
CMD ["service"]
