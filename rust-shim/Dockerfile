FROM rust:bookworm

WORKDIR /src

RUN rustup target add wasm32-wasip1

COPY Cargo.toml Cargo.lock ./
COPY src ./src

RUN cargo build --release --target wasm32-wasip1
