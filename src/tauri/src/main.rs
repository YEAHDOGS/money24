// Desktop entry point. The app itself lives in lib.rs, shared with Android.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    money24_lib::run()
}
