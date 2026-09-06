extern "C" {
    fn c_compute(x: i32) -> i32;
    static mut G_STATE: i32;
}

macro_rules! log_debug {
    ($($arg:tt)*) => {
        println!("[debug] {}", format!($($arg)*));
    };
}

pub enum Mode {
    Fast,
    Safe,
}

#[no_mangle]
pub extern "C" fn rust_entry(x: i32) -> i32 {
    unsafe { c_compute(x) }
}

fn main() {
    log_debug!("starting");
    let mode = Mode::Fast;
    match mode {
        Mode::Fast => println!("fast"),
        Mode::Safe => println!("safe"),
    }
}
