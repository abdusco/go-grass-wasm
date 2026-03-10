use std::path::PathBuf;
use std::slice;
use std::str;
use std::sync::Mutex;

use grass_compiler::{from_path, from_string, Options, OutputStyle};

static OUTPUT: Mutex<Vec<u8>> = Mutex::new(Vec::new());
static ERROR: Mutex<Vec<u8>> = Mutex::new(Vec::new());

const STYLE_EXPANDED: i32 = 0;
const STYLE_COMPRESSED: i32 = 1;

fn set_output(bytes: Vec<u8>) {
    if let Ok(mut out) = OUTPUT.lock() {
        *out = bytes;
    }
}

fn set_error(message: String) {
    if let Ok(mut err) = ERROR.lock() {
        *err = message.into_bytes();
    }
}

fn clear_error() {
    if let Ok(mut err) = ERROR.lock() {
        err.clear();
    }
}

fn parse_utf8(ptr: *const u8, len: usize, name: &str) -> Result<String, String> {
    if ptr.is_null() && len > 0 {
        return Err(format!("{name} pointer is null"));
    }

    let bytes = unsafe { slice::from_raw_parts(ptr, len) };
    str::from_utf8(bytes)
        .map(str::to_owned)
        .map_err(|e| format!("{name} is not valid UTF-8: {e}"))
}

fn parse_style(style: i32) -> Result<OutputStyle, String> {
    match style {
        STYLE_EXPANDED => Ok(OutputStyle::Expanded),
        STYLE_COMPRESSED => Ok(OutputStyle::Compressed),
        _ => Err(format!("invalid style value: {style}")),
    }
}

fn parse_include_dirs(ptr: *const u8, len: usize) -> Result<Vec<PathBuf>, String> {
    let raw = parse_utf8(ptr, len, "include_dirs")?;
    if raw.is_empty() {
        return Ok(Vec::new());
    }

    Ok(raw
        .lines()
        .filter(|line| !line.trim().is_empty())
        .map(PathBuf::from)
        .collect())
}

fn build_options(style: i32, include_ptr: *const u8, include_len: usize) -> Result<Options<'static>, String> {
    let output_style = parse_style(style)?;
    let include_dirs = parse_include_dirs(include_ptr, include_len)?;

    Ok(Options::default()
        .style(output_style)
        .load_paths(&include_dirs)
        .unicode_error_messages(true))
}

#[no_mangle]
pub extern "C" fn alloc(len: usize) -> *mut u8 {
    let mut buf = Vec::<u8>::with_capacity(len);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub extern "C" fn dealloc(ptr: *mut u8, len: usize) {
    if ptr.is_null() {
        return;
    }
    unsafe {
        let _ = Vec::from_raw_parts(ptr, len, len);
    }
}

#[no_mangle]
pub extern "C" fn compile_path(
    path_ptr: *const u8,
    path_len: usize,
    style: i32,
    include_ptr: *const u8,
    include_len: usize,
) -> i32 {
    clear_error();

    let path = match parse_utf8(path_ptr, path_len, "path") {
        Ok(v) => v,
        Err(e) => {
            set_error(e);
            return 1;
        }
    };

    let options = match build_options(style, include_ptr, include_len) {
        Ok(v) => v,
        Err(e) => {
            set_error(e);
            return 1;
        }
    };

    match from_path(path, &options) {
        Ok(css) => {
            set_output(css.into_bytes());
            0
        }
        Err(err) => {
            set_error(err.to_string());
            1
        }
    }
}

#[no_mangle]
pub extern "C" fn compile_string(
    src_ptr: *const u8,
    src_len: usize,
    style: i32,
    include_ptr: *const u8,
    include_len: usize,
) -> i32 {
    clear_error();

    let source = match parse_utf8(src_ptr, src_len, "source") {
        Ok(v) => v,
        Err(e) => {
            set_error(e);
            return 1;
        }
    };

    let options = match build_options(style, include_ptr, include_len) {
        Ok(v) => v,
        Err(e) => {
            set_error(e);
            return 1;
        }
    };

    match from_string(source, &options) {
        Ok(css) => {
            set_output(css.into_bytes());
            0
        }
        Err(err) => {
            set_error(err.to_string());
            1
        }
    }
}

#[no_mangle]
pub extern "C" fn get_output_ptr() -> *const u8 {
    OUTPUT
        .lock()
        .map(|out| out.as_ptr())
        .unwrap_or(std::ptr::null())
}

#[no_mangle]
pub extern "C" fn get_output_len() -> usize {
    OUTPUT.lock().map(|out| out.len()).unwrap_or(0)
}

#[no_mangle]
pub extern "C" fn get_error_ptr() -> *const u8 {
    ERROR
        .lock()
        .map(|err| err.as_ptr())
        .unwrap_or(std::ptr::null())
}

#[no_mangle]
pub extern "C" fn get_error_len() -> usize {
    ERROR.lock().map(|err| err.len()).unwrap_or(0)
}
