# Base image from Taurus
FROM blazemeter/taurus@sha256:24a709d114f7480517fd1552485a4b6e89c6268f0d8b62fb0f3dbf61e69caf57


# Install Locust library
RUN pip3 install 'locust==2.14.1'

# Copy any locust file and nested folder into /bzt-configs
COPY scripts /bzt-configs

# Setup workdir
WORKDIR /bzt-configs

# Sample of running a Taurus server
# CMD ["/usr/local/bin/bzt", "/taurus-configs/taurus.yaml", "-o", "modules.console.disable=true", "-o", "settings.artifacts-dir=/taurus-logs/%Y-%m-%d_%H-%M-%S.%f"]
