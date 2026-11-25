import Wallet from './Wallet.svelte';
// Shared desk styles: fluid type, springs, the live dot and list scrollbars.
import '../feed/feed.css';
import { mountProtected as mount } from '../lib/app-lock/mount.js';

export default mount(Wallet, { target: document.getElementById('wallet') });
