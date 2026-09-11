import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class RadarScreen extends StatefulWidget {
  const RadarScreen({super.key});

  @override
  State<RadarScreen> createState() => _RadarScreenState();
}

class _RadarScreenState extends State<RadarScreen>
    with SingleTickerProviderStateMixin {
  late AnimationController radarController;

  bool showNotification = false;

  @override
  void initState() {
    super.initState();

    radarController = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 4),
    )..repeat();

    // Simulate nearby match detection.
    Future.delayed(const Duration(seconds: 2), () {
      if (!mounted) return;

      setState(() {
        showNotification = true;
      });

      HapticFeedback.selectionClick();
    });
  }

  @override
  void dispose() {
    radarController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xFF121A29),
            Color(0xFF17243A),
            Color(0xFF17243A),
          ],
        ),
      ),

      child: SafeArea(
        child: Column(
          children: [

            // Pink Figma line
            Container(
              height: 2,
              width: double.infinity,
              color: const Color(0xFFE7839A),
            ),

            // Header
            Container(
              height: 62,
              padding: const EdgeInsets.symmetric(
                horizontal: 24,
              ),
              decoration: const BoxDecoration(
                border: Border(
                  bottom: BorderSide(
                    color: Color(0xFF657187),
                    width: 1,
                  ),
                ),
              ),

              child: Row(
                mainAxisAlignment:
                MainAxisAlignment.spaceBetween,

                children: [

                  const Text(
                    'EchoSync',
                    style: TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.bold,
                      color: Colors.white,
                    ),
                  ),

                  Row(
                    children: [

                      Container(
                        width: 7,
                        height: 7,
                        decoration:
                        const BoxDecoration(
                          color: Color(0xFFE8899D),
                          shape: BoxShape.circle,
                        ),
                      ),

                      const SizedBox(width: 7),

                      const Text(
                        'scanning',
                        style: TextStyle(
                          fontSize: 14,
                          color: Color(0xFFE0E5ED),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // Radar
            Expanded(
              child: Stack(
                children: [

                  Center(
                    child: AnimatedBuilder(
                      animation: radarController,
                      builder: (context, child) {
                        return CustomPaint(
                          size: const Size(
                            330,
                            390,
                          ),
                          painter: RadarPainter(
                            progress:
                            radarController.value,
                          ),
                        );
                      },
                    ),
                  ),

                  // Match card
                  const Positioned(
                    left: 28,
                    right: 28,
                    bottom: 18,
                    child: RadarMatchCard(),
                  ),

                  // Reactive notification
                  if (showNotification)
                    Positioned(
                      left: 24,
                      right: 24,
                      top: 22,
                      child: MatchNotification(
                        onClose: () {
                          setState(() {
                            showNotification = false;
                          });
                        },
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ============================================================
// MATCH NOTIFICATION
// ============================================================

class MatchNotification extends StatelessWidget {
  final VoidCallback onClose;

  const MatchNotification({
    super.key,
    required this.onClose,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,

      child: Container(
        padding: const EdgeInsets.all(16),

        decoration: BoxDecoration(
          color: const Color(0xFF202D40),
          border: Border.all(
            color: const Color(0xFF7C8A9D),
            width: .8,
          ),
          borderRadius: BorderRadius.circular(4),

          boxShadow: [
            BoxShadow(
              color: Colors.black.withOpacity(.35),
              blurRadius: 18,
            ),
          ],
        ),

        child: Row(
          children: [

            Container(
              width: 9,
              height: 9,
              decoration: const BoxDecoration(
                color: Color(0xFFE8899D),
                shape: BoxShape.circle,
              ),
            ),

            const SizedBox(width: 12),

            const Expanded(
              child: Column(
                crossAxisAlignment:
                CrossAxisAlignment.start,
                children: [

                  Text(
                    'Match nearby',
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                    ),
                  ),

                  SizedBox(height: 4),

                  Text(
                    'Rust Programming · ~6 m',
                    style: TextStyle(
                      fontSize: 11,
                      color: Color(0xFFB5C0D0),
                    ),
                  ),
                ],
              ),
            ),

            GestureDetector(
              onTap: onClose,
              child: const Icon(
                Icons.close,
                size: 17,
                color: Color(0xFF9AA6B7),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ============================================================
// RADAR MATCH CARD
// ============================================================

class RadarMatchCard extends StatelessWidget {
  const RadarMatchCard({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.only(top: 14),

      decoration: const BoxDecoration(
        border: Border(
          top: BorderSide(
            color: Color(0xFF778296),
            width: 1,
          ),
        ),
      ),

      child: const Column(
        crossAxisAlignment:
        CrossAxisAlignment.start,

        children: [

          Text(
            'haptic triggered · match nearby',
            style: TextStyle(
              fontSize: 11,
              color: Color(0xFF9BA5B5),
            ),
          ),

          SizedBox(height: 8),

          Text(
            'Rust Programming',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),

          SizedBox(height: 4),

          Text(
            'USR_7x2 · ~6 m away',
            style: TextStyle(
              fontSize: 11,
              color: Color(0xFF9BA5B5),
            ),
          ),
        ],
      ),
    );
  }
}

// ============================================================
// RADAR PAINTER
// ============================================================

class RadarPainter extends CustomPainter {
  final double progress;

  RadarPainter({
    required this.progress,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(
      size.width / 2,
      size.height / 2,
    );

    const radius = 130.0;

    // Glow
    final glowPaint = Paint()
      ..shader = RadialGradient(
        colors: [
          const Color(0xFFE7849B)
              .withOpacity(.14),
          Colors.transparent,
        ],
      ).createShader(
        Rect.fromCircle(
          center: center,
          radius: radius,
        ),
      );

    canvas.drawCircle(
      center,
      radius,
      glowPaint,
    );

    // Rings
    final circlePaint = Paint()
      ..color =
      const Color(0xFF9BAAC1)
          .withOpacity(.45)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;

    for (int i = 1; i <= 4; i++) {
      canvas.drawCircle(
        center,
        radius * i / 4,
        circlePaint,
      );
    }

    // //Grid
    final gridPaint = Paint()
      ..color =
      const Color(0xFF9BAAC1)
          .withOpacity(.28)
      ..strokeWidth = 1;

    canvas.drawLine(
      Offset(center.dx - radius, center.dy),
      Offset(center.dx + radius, center.dy),
      gridPaint,
    );

    canvas.drawLine(
      Offset(center.dx, center.dy - radius),
      Offset(center.dx, center.dy + radius),
      gridPaint,
    );

    // Diagonal
    canvas.drawLine(
      Offset(
        center.dx - radius * .7,
        center.dy - radius * .7,
      ),
      Offset(
        center.dx + radius * .7,
        center.dy + radius * .7,
      ),
      gridPaint,
    );

    canvas.drawLine(
      Offset(
        center.dx - radius * .7,
        center.dy + radius * .7,
      ),
      Offset(
        center.dx + radius * .7,
        center.dy - radius * .7,
      ),
      gridPaint,
    );

    // Scanning beam
    final angle = progress * math.pi * 2;

    final end = Offset(
      center.dx + math.cos(angle) * radius,
      center.dy + math.sin(angle) * radius,
    );

    final scanPaint = Paint()
      ..color = const Color(0xFFE990A5)
      ..strokeWidth = 1.2;

    canvas.drawLine(
      center,
      end,
      scanPaint,
    );

    // Center
    canvas.drawCircle(
      center,
      7,
      Paint()..color = const Color(0xFFE78A9F),
    );

    canvas.drawCircle(
      center,
      3,
      Paint()..color = const Color(0xFF182334),
    );

    // Target
    final target = Offset(
      center.dx + 78,
      center.dy - 48,
    );

    final targetRing = Paint()
      ..color = const Color(0xFFE2E8F0)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;

    canvas.drawCircle(
      target,
      23,
      targetRing,
    );

    canvas.drawCircle(
      target,
      9,
      Paint()..color = const Color(0xFFE2E8F0),
    );

    canvas.drawCircle(
      target,
      4,
      Paint()..color = const Color(0xFF667387),
    );

    // Dotted connection
    final distance =
        (target - center).distance;

    final direction =
        (target - center) / distance;

    final dashPaint = Paint()
      ..color = const Color(0xFFE1E6ED)
      ..strokeWidth = 1;

    for (
    double d = 15;
    d < distance - 20;
    d += 10
    ) {
      canvas.drawLine(
        center + direction * d,
        center + direction * (d + 5),
        dashPaint,
      );
    }
  }

  @override
  bool shouldRepaint(
      covariant RadarPainter oldDelegate,
      ) {
    return oldDelegate.progress != progress;
  }
}