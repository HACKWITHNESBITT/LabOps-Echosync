/// EchoSync backend client: WebSocket presence/radar/match stream + REST.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/echosync_models.dart';

/// Default dev backend for the Android emulator (10.0.2.2 -> host loopback).
/// Flutter web/desktop builds override `host` in main.dart with 127.0.0.1.
const String kDefaultApiHost = '10.0.2.2';
const int kDefaultApiPort = 8080;

class EchoSyncClient {
  final String userId;
  final String host;
  final int port;

  WebSocketChannel? _channel;
  StreamSubscription? _sub;
  final StreamController<WsFrame> _frames =
      StreamController<WsFrame>.broadcast();

  bool get isOpen => _channel != null;

  /// Frames coming straight off the wire (radar sweeps, match alerts, TTL pings).
  Stream<WsFrame> get frames => _frames.stream;

  EchoSyncClient({
    required this.userId,
    this.host = kDefaultApiHost,
    this.port = kDefaultApiPort,
  });

  static String baseUrl(String host, int port) => 'http://$host:$port';

  /// Connects and starts pumping frames. Reconnects with backoff on drop.
  void connect({Duration reconnectDelay = const Duration(seconds: 3)}) {
    if (_channel != null) return;
    _open(reconnectDelay);
  }

  void _open(Duration delay) {
    try {
      final wsUrl = 'ws://$host:$port/ws?user_id=$userId';
      _channel = WebSocketChannel.connect(Uri.parse(wsUrl));
      _sub = _channel!.stream.listen(
        (raw) => _frames.add(_parse(raw)),
        onDone: () {
          _teardown();
          _scheduleReconnect(delay);
        },
        onError: (_) {
          _teardown();
          _scheduleReconnect(delay);
        },
      );
      // Server pings are answered automatically by the channel's ping handler.
    } catch (_) {
      _teardown();
      _scheduleReconnect(delay);
    }
  }

  void _scheduleReconnect(Duration delay) {
    Timer(delay, () {
      if (_channel == null) connect(reconnectDelay: delay);
    });
  }

  /// Reports this device's location + scrubbed interest tokens.
  void sendPresence(double lat, double lng, List<String> interests) {
    _channel?.sink.add(jsonEncode({
      'type': 'presence',
      'lat': lat,
      'lng': lng,
      'interests': interests,
    }));
  }

  /// Asks the backend for the latest radar sweep state.
  void requestScan() {
    _channel?.sink.add(jsonEncode({'type': 'scan'}));
  }

  void dispose() {
    _teardown();
    _frames.close();
  }

  void _teardown() {
    _sub?.cancel();
    _sub = null;
    _channel?.sink.close();
    _channel = null;
  }

  WsFrame _parse(dynamic raw) {
    try {
      final Map<String, dynamic> json = jsonDecode(raw as String);
      switch (json['type']) {
        case 'hello':
          return HelloFrame(json['message'] ?? '');
        case 'match':
          return MatchFrame(
            EchoMatch.fromJson(json['match'] as Map<String, dynamic>),
            (json['latency_ms'] as num?)?.toInt(),
          );
        case 'radar':
          return RadarFrame(
            RadarState.fromJson(json['radar'] as Map<String, dynamic>),
          );
        case 'pong':
        case 'match_ttl':
          return const PingFrame();
        default:
          return const WsParseError('unknown frame type');
      }
    } catch (e) {
      return WsParseError(e.toString());
    }
  }

  /// REST: generate a fresh AI icebreaker for a live ephemeral match.
  Future<String> fetchIcebreaker({
    required String matchId,
    required String forUser,
  }) async {
    final uri = Uri.parse('${baseUrl(host, port)}/v1/icebreaker');
    final client = HttpClient();
    try {
      final req = await client
          .postUrl(uri)
          .timeout(const Duration(seconds: 12));
      req.headers.set('Content-Type', 'application/json');
      req.add(utf8.encode(jsonEncode({
        'user_id': forUser,
        'match_id': matchId,
      })));
      final resp = await req.close();
      final bodyText = await resp.transform(utf8.decoder).join();
      if (resp.statusCode != 200) {
        throw Exception('icebreaker http ${resp.statusCode}');
      }
      final body = jsonDecode(bodyText) as Map<String, dynamic>;
      return body['icebreaker'] ?? '';
    } finally {
      client.close();
    }
  }
}