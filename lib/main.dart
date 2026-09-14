import 'dart:async';

import 'package:flutter/material.dart';

import 'icebreaker_screen.dart';
import 'models/echosync_models.dart';
import 'radar_screen.dart';
import 'services/echosync_client.dart';

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
  Timer? _beacon;

  static const _demoInterests = [
    'RUST PROGRAMMING',
    'FORMULA 1',
    'MACHINE LEARNING',
    'HIKING',
  ];

  @override
  void initState() {
    super.initState();
    client = EchoSyncClient(userId: 'usr_demo');
    client.frames.listen((frame) {
      if (frame is MatchFrame && mounted) {
        latestMatchLatencyMs = frame.latencyMs;
        setState(() => latestMatch = frame.match);
      }
    });
    client.connect();

    // Stand-in for the on-device pipeline: after each scrub cycle the client
    // reports sanitized interest tokens + location to the proximity engine.
    _beacon = Timer.periodic(const Duration(seconds: 6), (_) {
      client.sendPresence(37.7749295, -122.4194155, _demoInterests);
    });
    client.sendPresence(37.7749295, -122.4194155, _demoInterests);
  }

  @override
  void dispose() {
    _beacon?.cancel();
    client.dispose();
    super.dispose();
  }

  void _select(int index) {
    setState(() => selectedIndex = index);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF111A29),

      body: selectedIndex == 0
          ? RadarScreen(
              client: client,
              latestMatch: latestMatch,
              matchLatencyMs: latestMatchLatencyMs,
            )
          : IcebreakerScreen(match: latestMatch, client: client),

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
//