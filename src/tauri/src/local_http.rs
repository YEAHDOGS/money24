// A tiny GET for the local services' health and status routes, standard library only: one HTTP/1.0
// request per connection, so the answer is never chunked and needs no parsing beyond the status line.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpStream};
use std::time::Duration;

const LOOPBACK: [u8; 4] = [127, 0, 0, 1];
const CONNECT_TIMEOUT: Duration = Duration::from_millis(800);
const READ_TIMEOUT: Duration = Duration::from_secs(3);
const HEADER_END: &[u8] = b"\r\n\r\n";
const STATUS_OK: &str = "200";
pub const HEALTH_PATH: &str = "/healthz";

fn address(port: u16) -> SocketAddr {
    SocketAddr::from((LOOPBACK, port))
}

/// Whether anything at all listens on 127.0.0.1:port.
pub fn listening(port: u16) -> bool {
    TcpStream::connect_timeout(&address(port), CONNECT_TIMEOUT).is_ok()
}

/// The body of GET path on 127.0.0.1:port when it answers 200; None for any other answer or none.
pub fn get(port: u16, path: &str) -> Option<Vec<u8>> {
    let mut stream = TcpStream::connect_timeout(&address(port), CONNECT_TIMEOUT).ok()?;
    stream.set_read_timeout(Some(READ_TIMEOUT)).ok()?;
    stream.set_write_timeout(Some(READ_TIMEOUT)).ok()?;
    // The runner only answers to its own Host names (a guard against DNS rebinding), 127.0.0.1:port among them.
    write!(stream, "GET {path} HTTP/1.0\r\nHost: 127.0.0.1:{port}\r\nAccept: application/json\r\n\r\n").ok()?;
    let mut raw = Vec::new();
    stream.read_to_end(&mut raw).ok()?;
    let split = raw.windows(HEADER_END.len()).position(|w| w == HEADER_END)?;
    let status = std::str::from_utf8(&raw[..split]).ok()?.split_whitespace().nth(1)?;
    if status != STATUS_OK {
        return None;
    }
    Some(raw.split_off(split + HEADER_END.len()))
}

/// GET path as JSON, None when it does not answer 200 with JSON.
pub fn get_json(port: u16, path: &str) -> Option<serde_json::Value> {
    serde_json::from_slice(&get(port, path)?).ok()
}

/// Whether the service on port answers its health check.
pub fn healthy(port: u16) -> bool {
    get(port, HEALTH_PATH).is_some()
}
