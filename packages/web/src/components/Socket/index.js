import { io } from 'socket.io-client';
import { getBasePath } from '@/utils/base-path';

class WS {
  static instance;
  socket;

  constructor(url) {
    // socket.io's default endpoint is the root-absolute "/socket.io". Under a
    // sub-path the reverse proxy forwards "{basePath}socket.io", so set the
    // client path accordingly. At the site root this is exactly "/socket.io".
    const path = `${getBasePath()}socket.io`;
    this.socket = io(url, { path });

    this.socket.on('connect', () => {
    });

    this.socket.on('disconnect', () => {
    });
  }

  static getInstance(url) {
    if (!WS.instance) {
      WS.instance = new WS(url);
    }
    return WS.instance;
  }

  getSocket() {
    return this.socket;
  }
}

export default WS;
