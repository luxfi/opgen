// Code generated from vault's typed ops by opgen. DO NOT EDIT.
//
// vault's API.
//
// Every field is `#[serde(default)]`: the service is written in Go, where a
// struct field is always present and an absent one arrives as its zero value.
// A client that made absence an error would refuse answers the service
// considers complete.

#![allow(clippy::all)]

use serde::{Deserialize, Serialize};

/// One answer, as bytes.
pub struct Reply {
    pub status: u16,
    pub body: Vec<u8>,
}

/// How the bytes travel. The SDK knows the contract; the program it lands in
/// already has an HTTP client, and this is where it plugs in.
///
/// `target` is an absolute path with its query string, so the base
/// address belongs to the implementation and not to the call.
pub trait Transport {
    fn send(&self, method: &str, target: &str, body: Option<&[u8]>) -> Result<Reply, Error>;
}

/// A borrowed transport is a transport, so one client can be handed a reference
/// to a transport the program already owns.
impl<T: Transport + ?Sized> Transport for &T {
    fn send(&self, method: &str, target: &str, body: Option<&[u8]>) -> Result<Reply, Error> {
        (**self).send(method, target, body)
    }
}

/// What can go wrong, told apart by whose fault it is.
#[derive(Clone, Debug, PartialEq)]
pub enum Error {
    /// The bytes did not get there.
    Transport(String),
    /// They got there and the service refused.
    Status { status: u16, body: String },
    /// A value would not go on or come off the wire.
    Encoding(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::Transport(m) => write!(f, "transport: {m}"),
            Error::Status { status, body } => write!(f, "status {status}: {body}"),
            Error::Encoding(m) => write!(f, "encoding: {m}"),
        }
    }
}

impl std::error::Error for Error {}

/// Percent-encodes one path segment or query value. Everything outside the
/// unreserved set is escaped, which is correct in both places.
fn encode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.as_bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'.' | b'_' | b'~' => {
                out.push(*b as char)
            }
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Held {
    #[serde(default)]
    pub more: bool,
    #[serde(default)]
    pub secrets: Vec<Secret>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Party {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub org: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Ready {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub ready: bool,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Seal {
    #[serde(default)]
    pub at: String,
    #[serde(default)]
    pub blob: String,
    #[serde(default)]
    pub bytes: i64,
    #[serde(default)]
    pub counts: Vec<i32>,
    #[serde(default)]
    pub free: serde_json::Value,
    #[serde(default)]
    pub labels: std::collections::BTreeMap<String, String>,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub nested: Party,
    #[serde(default)]
    pub owner: Party,
    #[serde(default)]
    pub rotate: bool,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub weight: f64,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Sealed {
    #[serde(default)]
    pub at: String,
    #[serde(rename = "ref", default)]
    pub ref_: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Secret {
    #[serde(default)]
    pub at: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub owner: Party,
}

/// List the secrets this vault holds
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct VaultList {
    #[serde(default)]
    pub org: String,
    #[serde(default)]
    pub limit: i64,
}

/// Read one secret's record by name
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct VaultRead {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub reveal: bool,
}

/// vault's operations.
pub struct Client<T: Transport> {
    transport: T,
}

impl<T: Transport> Client<T> {
    /// Returns a client that calls over `transport`.
    pub fn new(transport: T) -> Self {
        Client { transport }
    }

    /// Report whether this replica is serving
    ///
    /// `GET /v1/health`
    pub fn vault_health(&self) -> Result<Ready, Error> {
        let target = String::from("/v1/health");
        let reply = self.transport.send("GET", &target, None)?;
        if reply.status < 200 || reply.status >= 300 {
            return Err(Error::Status {
                status: reply.status,
                body: String::from_utf8_lossy(&reply.body).into_owned(),
            });
        }
        serde_json::from_slice(&reply.body).map_err(|e| Error::Encoding(e.to_string()))
    }

    /// List the secrets this vault holds
    ///
    /// `GET /v1/secrets`
    pub fn vault_list(&self, input: &VaultList) -> Result<Held, Error> {
        let mut target = String::from("/v1/secrets");
        target.push_str("?org=");
        target.push_str(&encode(&input.org));
        target.push_str("&limit=");
        target.push_str(&encode(&input.limit.to_string()));
        let reply = self.transport.send("GET", &target, None)?;
        if reply.status < 200 || reply.status >= 300 {
            return Err(Error::Status {
                status: reply.status,
                body: String::from_utf8_lossy(&reply.body).into_owned(),
            });
        }
        serde_json::from_slice(&reply.body).map_err(|e| Error::Encoding(e.to_string()))
    }

    /// Read one secret's record by name
    ///
    /// `GET /v1/secrets/{name}`
    pub fn vault_read(&self, input: &VaultRead) -> Result<Secret, Error> {
        let mut target = String::new();
        target.push_str("/v1/secrets/");
        target.push_str(&encode(&input.name));
        target.push_str("?reveal=");
        target.push_str(&encode(&input.reveal.to_string()));
        let reply = self.transport.send("GET", &target, None)?;
        if reply.status < 200 || reply.status >= 300 {
            return Err(Error::Status {
                status: reply.status,
                body: String::from_utf8_lossy(&reply.body).into_owned(),
            });
        }
        serde_json::from_slice(&reply.body).map_err(|e| Error::Encoding(e.to_string()))
    }

    /// Seal a secret. The value is never readable again.
    ///
    /// `POST /v1/seal`
    pub fn vault_seal(&self, input: &Seal) -> Result<Sealed, Error> {
        let target = String::from("/v1/seal");
        let body = serde_json::to_vec(input).map_err(|e| Error::Encoding(e.to_string()))?;
        let reply = self.transport.send("POST", &target, Some(&body))?;
        if reply.status < 200 || reply.status >= 300 {
            return Err(Error::Status {
                status: reply.status,
                body: String::from_utf8_lossy(&reply.body).into_owned(),
            });
        }
        serde_json::from_slice(&reply.body).map_err(|e| Error::Encoding(e.to_string()))
    }
}
