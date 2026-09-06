use std::collections::HashMap;
use std::fmt::{self, Display};

const MAX_CONNECTIONS: usize = 256;
static DEFAULT_PORT: u16 = 8080;

/// Server configuration.
pub struct Config {
    pub name: String,
    pub port: u16,
    extras: HashMap<String, String>,
}

pub trait Describe {
    fn describe(&self) -> String;
}

impl Describe for Config {
    fn describe(&self) -> String {
        format!("{} on port {}", self.name, self.port)
    }
}

impl Config {
    pub fn new(name: String, port: u16) -> Config {
        Config {
            name,
            port,
            extras: HashMap::new(),
        }
    }

    pub fn set_extra(&mut self, key: String, value: String) {
        self.extras.insert(key, value);
    }
}

impl fmt::Display for Config {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "{}", self.describe())
    }
}

pub fn load_default() -> Config {
    Config::new("default".to_string(), DEFAULT_PORT)
}
