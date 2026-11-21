FROM gcr.io/distroless/base-debian12:nonroot
COPY dist/authz /authz
ENTRYPOINT ["/authz"]
CMD ["migrate"]
