import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';

import 'icebreaker_screen.dart';
import 'models/echosync_models.dart';
import 'radar_screen.dart';
import 'services/echosync_client.dart';
import 'services/mic_streamer.dart';

void main() {
  runApp(const EchoSyncApp());
}

class EchoSyncApp extends StatelessWidget {
  const EchoSyncApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'EchoSync',
      theme: ThemeData(
        brightness: Brightness.dark,
        scaffoldBackgroundColor: const Color(0xFF111A29),
        fontFamily: 'Arial',
      ),
      home: const EchoSyncHome(),
    );
  }
}

class EchoSyncHome extends StatefulWidget {
  const EchoSyncHome({super.key});

  @override
  State<EchoSyncHome> createState() => _EchoSyncHomeState();
}

class _EchoSyncHomeState extends State<EchoSyncHome> {
  int selectedIndex = 0;
  late final EchoSyncClient client;
  EchoMatch? latestMatch;
  int? latestMatchLatencyMs;
  MicStreamer? _mic;
  String _liveText = '';
  bool get _micLive => _mic?.isStreaming ?? false;
  late final String _apiHost;
  late final String _userId;

  @override
  void initState() {
    super.initState();
    // Web/desktop dev reaches the local backend at 127.0.0.1; the Android
    // emulator maps host loopback to 10.0.2.2.
    _apiHost = kIsWeb ? '127.0.0.1' : kDefaultApiHost;
    _userId = 'usr_${math.Random().nextInt(90000) + 10000}';
    client = EchoSyncClient(userId: _userId, host: _apiHost);
    client.frames.listen((frame) {
      if (frame is MatchFrame && mounted) {
        latestMatchLatencyMs = frame.latencyMs;
        setState(() => latestMatch = frame.match);
      }
    });
    client.connect();

    _mic = MicStreamer(
      userId: client.userId,
      host: _apiHost,
      port: client.port,
      lat: 37.7749295,
      lng: -122.4194155,
    );
    _mic!.transcripts.listen((t) {
      if (t.isNotEmpty && mounted) setState(() => _liveText = t);
    });
  }

  @override
  void dispose() {
    _mic?.dispose();
    client.dispose();
    super.dispose();
  }

  void _select(int index) {
    setState(() => selectedIndex = index);
  }

  // Live caption overlay fed by the mic -> backend audio stream.
  Widget _liveCaption() {
    if (!_micLive && _liveText.isEmpty) return const SizedBox.shrink();
    return Align(
      alignment: Alignment.bottomCenter,
      child: Container(
        margin: const EdgeInsets.only(bottom: 76, left: 24, right: 24),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        decoration: BoxDecoration(
          color: const Color(0xFF16233A).withValues(alpha: 0.92),
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: const Color(0xFF27405F)),
        ),
        child: Row(
          children: [
            Icon(_micLive ? Icons.graphic_eq : Icons.mic_off,
                color: _micLive ? const Color(0xFF2FE6A7) : const Color(0xFF7C8797),
                size: 18),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                _micLive && _liveText.isEmpty ? 'listening…' : _liveText,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 13, color: Color(0xFFE1E6ED)),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _toggleMic() async {
    final mic = _mic;
    if (mic?.isStreaming ?? false) {
      setState(() {});
      await mic!.stop();
      _liveText = 'capture stopped';
      return;
    }
    if (!mic!.supportedOnPlatform) {
      _showToast('Microphone streaming needs a device build '
          '(emulator/desktop); radar demo still works here.');
      return;
    }
    try {
      await mic.start();
      setState(() {});
    } catch (e) {
      _showToast('Mic error: $e');
    }
  }

  void _showToast(String msg) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(
        content: Text(msg),
        behavior: SnackBarBehavior.floating,
        backgroundColor: const Color(0xFF27405F),
      ));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF111A29),

      body: Stack(
        children: [
          selectedIndex == 0
              ? RadarScreen(
                  client: client,
                  latestMatch: latestMatch,
                  matchLatencyMs: latestMatchLatencyMs,
                  host: _apiHost,
                )
              : IcebreakerScreen(match: latestMatch, client: client),
          _liveCaption(),
        ],
      ),

      floatingActionButton: FloatingActionButton(
        heroTag: 'mic',
        backgroundColor: _micLive ? const Color(0xFF2FE6A7) : const Color(0xFF27405F),
        foregroundColor: _micLive ? const Color(0xFF0A111C) : const Color(0xFFE1E6ED),
        shape: const CircleBorder(),
        onPressed: _toggleMic,
        child: Icon(_micLive ? Icons.mic : Icons.mic_none),
      ),

      bottomNavigationBar: BottomNavigationBar(
        currentIndex: selectedIndex,
        onTap: _select,

        backgroundColor: const Color(0xFF111A29),
        elevation: 0,
        type: BottomNavigationBarType.fixed,

        selectedItemColor: const Color(0xFFE1E6ED),
        unselectedItemColor: const Color(0xFF7C8797),

        selectedFontSize: 11,
        unselectedFontSize: 11,

        showSelectedLabels: true,
        showUnselectedLabels: true,

        items: const [
          BottomNavigationBarItem(icon: SizedBox.shrink(), label: 'Radar'),
          BottomNavigationBarItem(icon: SizedBox.shrink(), label: 'Matches'),
        ],
      ),
    );
  }
}