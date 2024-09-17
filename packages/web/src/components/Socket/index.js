import { io } from 'socket.io-client';

class WS {
  static instance;
  socket;

  constructor(url) {
    this.socket = io(url);

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
