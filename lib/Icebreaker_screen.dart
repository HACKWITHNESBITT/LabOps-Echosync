import 'package:flutter/material.dart';

class IcebreakerScreen extends StatelessWidget {
  const IcebreakerScreen({super.key});

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

            // Pink line
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

              child: const Row(
                mainAxisAlignment:
                MainAxisAlignment.spaceBetween,

                children: [

                  Text(
                    'EchoSync',
                    style: TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.bold,
                    ),
                  ),

                  Text(
                    '00:00',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),

            // Content
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(
                  28,
                  36,
                  28,
                  20,
                ),

                child: Column(
                  crossAxisAlignment:
                  CrossAxisAlignment.start,

                  children: [

                    const Text(
                      'shared interest',
                      style: TextStyle(
                        fontSize: 13,
                        color: Color(0xFFB5C0D0),
                      ),
                    ),

                    const SizedBox(height: 14),

                    const Text(
                      'Rust Programming',
                      style: TextStyle(
                        fontSize: 31,
                        fontWeight: FontWeight.w400,
                        letterSpacing: -1,
                      ),
                    ),

                    const SizedBox(height: 14),

                    Row(
                      children: [

                        Expanded(
                          child: Container(
                            height: 1,
                            color:
                            const Color(0xFFB3BDCD),
                          ),
                        ),

                        const SizedBox(width: 14),

                        const Text(
                          'similarity 0.943',
                          style: TextStyle(
                            fontSize: 12,
                            color: Color(0xFFD0D7E1),
                          ),
                        ),
                      ],
                    ),

                    const SizedBox(height: 48),

                    const Text(
                      'icebreaker · on-device',
                      style: TextStyle(
                        fontSize: 13,
                        color: Color(0xFFB5C0D0),
                      ),
                    ),

                    const SizedBox(height: 28),

                    const Text(
                      '"Ask them what their favorite Rust '
                          'crate is for async networking and why '
                          'they prefer it over Go."',

                      style: TextStyle(
                        fontSize: 18,
                        height: 1.55,
                        fontWeight: FontWeight.w600,
                        color: Color(0xFFE8EBEF),
                      ),
                    ),

                    const Spacer(),

                    Container(
                      height: 1,
                      color: const Color(0xFF788598),
                    ),

                    const SizedBox(height: 16),

                    SizedBox(
                      width: double.infinity,
                      height: 52,

                      child: OutlinedButton(
                        onPressed: () {
                          ScaffoldMessenger.of(
                            context,
                          ).showSnackBar(
                            const SnackBar(
                              content: Text(
                                'Hello sent 👋',
                              ),
                            ),
                          );
                        },

                        style:
                        OutlinedButton.styleFrom(
                          side: const BorderSide(
                            color: Color(0xFFD0D7E1),
                          ),

                          shape:
                          const RoundedRectangleBorder(
                            borderRadius:
                            BorderRadius.zero,
                          ),
                        ),

                        child: const Text(
                          'say hello',
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    ),

                    const SizedBox(height: 12),

                    const Center(
                      child: Text(
                        'connection expires in 00:00',
                        style: TextStyle(
                          fontSize: 10,
                          color: Color(0xFF7E899A),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}