use crate::flow::Analyzer;
use crate::model::{Context, ValueSet};
use std::collections::BTreeMap;

pub(crate) fn analyze(context: &mut Context) {
    for _ in 0..16 {
        let mut returns = BTreeMap::<String, ValueSet>::new();
        let mut parameter_updates = BTreeMap::<(String, usize), ValueSet>::new();
        let mut capture_updates = BTreeMap::<(String, String), ValueSet>::new();
        for function in context.functions.values() {
            let result = Analyzer::new(context, function).run();
            returns
                .entry(function.id.clone())
                .or_default()
                .merge(&result.return_value);
            for (key, value) in result.parameter_updates {
                parameter_updates.entry(key).or_default().merge(&value);
            }
            for (key, value) in result.capture_updates {
                capture_updates.entry(key).or_default().merge(&value);
            }
        }
        let mut changed = returns != context.return_values;
        for (key, value) in parameter_updates {
            changed |= context
                .propagated_parameters
                .entry(key)
                .or_default()
                .merge(&value);
        }
        for (key, value) in capture_updates {
            changed |= context
                .propagated_captures
                .entry(key)
                .or_default()
                .merge(&value);
        }
        if !changed {
            break;
        }
        context.return_values = returns;
    }
    let calls: Vec<_> = context
        .functions
        .values()
        .map(|function| {
            (
                function.id.clone(),
                Analyzer::new(context, function).run().calls,
            )
        })
        .collect();
    for (owner, events) in calls {
        for event in events {
            emit_call(context, &owner, event);
        }
    }
}

fn emit_call(context: &mut Context, owner: &str, event: crate::call_model::CallEvent) {
    if event.resolution.targets.is_empty() {
        context.facts.add_unresolved(
            owner,
            "calls",
            &event.expression,
            event.resolution.reason.unwrap_or("dynamic-target"),
            event.span,
        );
        return;
    }
    let relation = if event.resolution.possible || event.resolution.targets.len() > 1 {
        "possible-calls"
    } else {
        "calls"
    };
    for target in event.resolution.targets {
        context
            .facts
            .add_edge(owner, &target, relation, event.span.clone());
    }
}
