{
  description = "mentat: personal assistant daemon — Claude as the brain, your infrastructure as the body";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};

    mentatd = pkgs.buildNpmPackage {
      pname = "mentatd";
      version = "3.0.0";
      src = self;

      npmDepsHash = "sha256-paKWa25wqIqRiMTvYe+nPEEXE1FPspBN1eXJs1oK76w=";

      # Install-time npm must agree with the runtime wrappers below.
      nodejs = pkgs.nodejs_24;

      # No build step: Node 24 runs the TypeScript sources directly via
      # type stripping (the repo has no compile output by design).
      dontNpmBuild = true;

      nativeBuildInputs = [ pkgs.makeWrapper ];

      installPhase = ''
        runHook preInstall
        npm prune --omit=dev
        mkdir -p $out/lib/mentat/scripts
        cp -r src node_modules package.json $out/lib/mentat/
        cp scripts/daily-reminder.ts $out/lib/mentat/scripts/
        makeWrapper ${pkgs.nodejs_24}/bin/node $out/bin/mentatd \
          --add-flags $out/lib/mentat/src/main.ts
        makeWrapper ${pkgs.nodejs_24}/bin/node $out/bin/mentat-reminder \
          --add-flags $out/lib/mentat/scripts/daily-reminder.ts
        runHook postInstall
      '';

      meta = {
        description = "Personal assistant daemon supervising Claude Code sessions";
        mainProgram = "mentatd";
      };
    };
    # Python interpreter with livekit-agents + the Silero VAD plugin, for the
    # voice agent. Built from upstream wheels; see nix/voice-env.nix.
    voice-env = import ./nix/voice-env.nix { inherit pkgs; };
  in {
    packages.${system} = {
      mentatd = mentatd;
      default = mentatd;
      voice-env = voice-env;
    };

    nixosModules.default = import ./nix/module.nix {
      mentatdPackage = mentatd;
      voiceEnvPackage = voice-env;
    };

    checks.${system} = {
      build = mentatd;

      # Pure-eval smoke test of the NixOS module: instantiating the config
      # forces every option default and the env-assembly logic without
      # building a system. Wrong types, missing reads, or bad merges fail
      # here at `nix flake check` time.
      module-eval = let
        lib = nixpkgs.lib;

        # What ultraviolet already deploys; each case below adds to it, so a
        # regression in the base config fails every case at once.
        base = {
          enable = true;
          claudePackage = pkgs.hello; # any package with a bin; eval-only
          environmentFile = "/run/agenix/mentat-env";
          maxBudgetUsd = 2.0;
          mcpConfig.shimmer = {
            type = "http";
            url = "http://127.0.0.1:8001/mcp";
          };
          reminder.enable = true;
        };

        evalMentat = extra: (lib.nixosSystem {
          inherit system;
          modules = [
            self.nixosModules.default
            { services.mentat = base // extra; }
          ];
        }).config;

        deployed = evalMentat { };
        withVoice = evalMentat {
          voice = {
            enable = true;
            environmentFile = "/run/agenix/mentat-voice-env";
          };
        };

        observed = {
          daemonEnv = deployed.systemd.services.mentatd.environment;
          reminderEnv = deployed.systemd.services.mentat-reminder.environment;
          reminderTimer = deployed.systemd.timers.mentat-reminder.timerConfig;
          voiceEnv = withVoice.systemd.services.mentat-voice.environment;
          voiceExecStart = withVoice.systemd.services.mentat-voice.serviceConfig.ExecStart;
        };
      in
      # The voice sub-block defaults OFF: the config ultraviolet deploys today
      # must gain no unit until it opts in explicitly.
      assert lib.assertMsg (!(deployed.systemd.services ? mentat-voice))
        "services.mentat.voice must default off; enabling the daemon rendered mentat-voice";
      # Opting in then needs nothing but the secrets file: the URLs on both
      # sides of the agent come from the module's own defaults, and the unit
      # runs the agent as a livekit worker.
      assert lib.assertMsg (observed.voiceEnv.LIVEKIT_URL == "ws://127.0.0.1:7880")
        "voice LIVEKIT_URL default changed: ${observed.voiceEnv.LIVEKIT_URL}";
      assert lib.assertMsg (observed.voiceEnv.MENTAT_URL == "http://127.0.0.1:8484")
        "voice MENTAT_URL default changed: ${observed.voiceEnv.MENTAT_URL}";
      assert lib.assertMsg (lib.hasSuffix "/agent.py start" observed.voiceExecStart)
        "voice unit does not start the agent worker: ${observed.voiceExecStart}";

      pkgs.runCommand "mentat-module-eval" {} ''
        cat > $out <<'EOF'
        ${builtins.toJSON observed}
        EOF
      '';
    };
  };
}
