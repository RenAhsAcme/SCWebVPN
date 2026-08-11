const highWaterMark = 4 << 20;
const lowWaterMark = 1 << 20;

export type WispTransportStreams = {
  read: ReadableStream<ArrayBuffer>;
  write: WritableStream<Uint8Array>;
};

export class RTCWispTransport {
  readonly streams: WispTransportStreams;

  private readController: ReadableStreamDefaultController<ArrayBuffer> | null = null;
  private pending: ArrayBuffer[] = [];
  private closed = false;

  constructor(private readonly channel: RTCDataChannel) {
    channel.binaryType = 'arraybuffer';
    channel.bufferedAmountLowThreshold = lowWaterMark;
    channel.addEventListener('message', (event) => this.receive(event.data));
    channel.addEventListener('close', () => this.close());
    channel.addEventListener('error', () => this.abort(new Error('RTCDataChannel failed')));

    const read = new ReadableStream<ArrayBuffer>({
      start: (controller) => {
        this.readController = controller;
        for (const packet of this.pending) controller.enqueue(packet);
        this.pending = [];
      },
      cancel: () => this.channel.close(),
    });
    const write = new WritableStream<Uint8Array>({
      write: async (packet) => {
        while (this.channel.bufferedAmount > highWaterMark) {
          await event(this.channel, 'bufferedamountlow', 'RTCDataChannel closed');
        }
        if (this.channel.readyState !== 'open') {
          throw new Error('RTCDataChannel is not open');
        }
        this.channel.send(packet.slice().buffer as ArrayBuffer);
      },
      close: () => this.channel.close(),
      abort: () => this.channel.close(),
    });
    this.streams = { read, write };
  }

  factory = (): WispTransportStreams => this.streams;

  private receive(value: unknown) {
    if (this.closed) return;
    if (value instanceof ArrayBuffer) {
      this.enqueue(value);
    } else if (ArrayBuffer.isView(value)) {
      this.enqueue(new Uint8Array(value.buffer, value.byteOffset, value.byteLength).slice().buffer);
    } else {
      this.abort(new TypeError('Wisp transport received a non-binary packet'));
    }
  }

  private enqueue(packet: ArrayBuffer) {
    if (this.readController) this.readController.enqueue(packet);
    else this.pending.push(packet);
  }

  private close() {
    if (this.closed) return;
    this.closed = true;
    this.readController?.close();
  }

  private abort(error: Error) {
    if (this.closed) return;
    this.closed = true;
    this.readController?.error(error);
  }
}

function event(target: RTCDataChannel, name: string, closed: string): Promise<void> {
  return new Promise((resolve, reject) => {
    target.addEventListener(name, () => resolve(), { once: true });
    target.addEventListener('close', () => reject(new Error(closed)), { once: true });
  });
}
