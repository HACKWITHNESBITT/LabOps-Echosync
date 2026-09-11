import 'dart:math' as math;
import 'package:echo_sync/radar_screen.dart';
import 'package:flutter/material.dart';

import 'Icebreaker_screen.dart';

void main() {
  runApp(const EchoSyncApp());
}

class EchoSyncApp extends StatelessWidget {
  const EchoSyncApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        brightness: Brightness.dark,
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
  int currentIndex = 0;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF111A29),

      body: currentIndex == 0
          ? const RadarScreen()
          : const IcebreakerScreen(),

      bottomNavigationBar: BottomNavigationBar(
        currentIndex: currentIndex,
        onTap: (index) {
          setState(() {
            currentIndex = index;
          });
        },
        backgroundColor: const Color(0xFF111A29),
        elevation: 0,
        type: BottomNavigationBarType.fixed,

        selectedItemColor: const Color(0xFFE0E5ED),
        unselectedItemColor: const Color(0xFF7E8796),

        selectedFontSize: 11,
        unselectedFontSize: 11,

        showSelectedLabels: true,
        showUnselectedLabels: true,

        items: const [
          BottomNavigationBarItem(
            icon: SizedBox.shrink(),
            label: 'Radar',
          ),
          BottomNavigationBarItem(
            icon: SizedBox.shrink(),
            label: 'Matches',
          ),
        ],
      ),
    );
  }
}