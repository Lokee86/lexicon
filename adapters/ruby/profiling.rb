# frozen_string_literal: true

require "fileutils"
require "json"

module LexiconRuby
  module Profiling
    module_function

    def measure(name)
      return yield if profile_path.empty?

      started = Process.clock_gettime(Process::CLOCK_MONOTONIC, :nanosecond)
      yield
    ensure
      if !profile_path.empty? && started
        phases << { "name" => name, "duration_ns" => Process.clock_gettime(Process::CLOCK_MONOTONIC, :nanosecond) - started }
      end
    end

    def set(name, value)
      counts[name] = value.to_i unless profile_path.empty?
    end

    def write
      return if profile_path.empty?

      FileUtils.mkdir_p(File.dirname(profile_path))
      File.write(
        profile_path,
        JSON.pretty_generate("version" => 1, "phases" => phases, "counts" => counts) + "\n"
      )
    end

    def profile_path
      @profile_path ||= ENV.fetch("LEXICON_ADAPTER_PROFILE", "")
    end

    def phases
      @phases ||= []
    end

    def counts
      @counts ||= {}
    end
  end
end
