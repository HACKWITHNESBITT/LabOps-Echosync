/// Live microphone -> EchoSync audio/stream relay.
///
/// Captures mono pcm_s16le @ 16 kHz (native mobile/desktop only; the web
/// target uses MediaRecorder which cannot emit raw PCM, so mic streaming is
/// intentionally disabled there) and pushes the frames as binary frames to
/// `WS /v1/audio/stream`. Live partial/final transcripts come back over the
/// same socket and are surfaced to the UI.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:record/record.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class MicStreamer {
  final String userId;
  final String host;
  final int port;
  final double lat;
  final double lng;

  WebSocketChannel? _channel;
  StreamSubscription<Uint8List>? _sub;
  AudioRecorder? _recorder;
  StreamSubscription<dynamic>? _rx;
  bool _active = false;

  final StreamController<String> _transcripts =
      StreamController<String>.broadcast();

  /// Screens live speech (partial + final) while the mic is hot.
  Stream<String> get transcripts => _transcripts.stream;

  bool get isStreaming => _active;
  bool get supportedOnPlatform => !kIsWeb;

  MicStreamer({
    required this.userId,
    required this.host,
    required this.port,
    required this.lat,
    required this.lng,
  });

  /// Starts capture and pumps PCM frames to the backend audio stream.
  Future<void> start() async {
    if (_active) return;
    if (kIsWeb) {
      throw UnsupportedError(
          'Mic streaming needs a device build (web cannot emit raw PCM).');
    }
    if (!await AudioRecorder().hasPermission()) {
      throw Exception('Microphone permission denied');
    }
    _recorder = AudioRecorder();
    final wsUrl =
        'ws://$host:$port/v1/audio/stream?user_id=$userId&lat=$lat&lng=$lng';
    _channel = WebSocketChannel.connect(Uri.parse(wsUrl));
    _rx = _channel!.stream.listen((raw) {
      if (raw is String) {
        try {
          final s = jsonDecode(raw) as Map<String, dynamic>;
          final type = s['type'] as String?;
          if (type == 'partial' || type == 'final') {
            _transcripts.add('${s['transcript'] ?? ''}');
          }
        } catch (_) {}
      }
    });

    _channel!.sink.add(jsonEncode({'type': 'start', 'lat': lat, 'lng': lng}));

    const config = RecordConfig(
      encoder: AudioEncoder.pcm16bits,
      sampleRate: 16000,
      numChannels: 1,
      autoGain: true,
      echoCancel: true,
      noiseSuppress: true,
    );
    final stream = await _recorder!.startStream(config);
    _sub = stream.listen(
      (chunk) => _channel?.sink.add(chunk),
      onError: (_) => stop(),
    );
    _active = true;
  }

  /// Stops capture, asks the backend to finalize the utterance, and closes.
  Future<void> stop() async {
    if (!_active) return;
    _active = false;
    try {
      await _sub?.cancel();
      _sub = null;
      await _recorder?.stop();
      _recorder = null;
    } catch (_) {}
    try {
      _channel?.sink.add(jsonEncode({'type': 'flush'}));
      _channel?.sink.add(jsonEncode({'type': 'stop'}));
    } catch (_) {}
    await _rx?.cancel();
    _rx = null;
    _channel?.sink.close();
    _channel = null;
  }

  void dispose() {
    stop();
    _transcripts.close();
  }
}