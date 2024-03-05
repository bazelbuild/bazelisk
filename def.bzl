def _sha256sum_impl(ctx):
  ctx.actions.run_shell(
    outputs = [ctx.outputs.sha256],
    inputs = [ctx.file.src],
    command = "CWD=$PWD && cd `dirname {0}` && sha256sum `basename {0}` > $CWD/{1}".format(ctx.file.src.path, ctx.outputs.sha256.path),
  )

  return DefaultInfo(files = depset([ctx.outputs.sha256]))

_sha256sum = rule(
  implementation = _sha256sum_impl,
  attrs = {
    "src": attr.label(mandatory = True, allow_single_file = True),
    "sha256": attr.output(),
  }
)

def sha256sum(name, src, **kwargs):
  """
  Macro to create a sha256 message digest file
  """
  _sha256sum(
    name = name,
    src = src,
    sha256 = "%s.sha256" % src,
    **kwargs,
  )
