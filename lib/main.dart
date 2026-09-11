import 'package:flutter/material.dart';
import 'radar_screen.dart';
import 'icebreaker_screen.dart';

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

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF111A29),

      body: selectedIndex == 0
          ? const RadarScreen()
          : const IcebreakerScreen(),

      bottomNavigationBar: BottomNavigationBar(
        currentIndex: selectedIndex,
        onTap: (index) {
          setState(() {
            selectedIndex = index;
          });
        },

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