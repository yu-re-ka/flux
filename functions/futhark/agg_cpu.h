/*
 * Headers
*/

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>


/*
 * Initialisation
*/

struct futhark_cpu_context_config ;
struct futhark_cpu_context_config *futhark_cpu_context_config_new();
void futhark_cpu_context_config_free(struct futhark_cpu_context_config *cfg);
void futhark_cpu_context_config_set_debugging(struct futhark_cpu_context_config *cfg,
                                          int flag);
void futhark_cpu_context_config_set_logging(struct futhark_cpu_context_config *cfg,
                                        int flag);
struct futhark_cpu_context ;
struct futhark_cpu_context *futhark_cpu_context_new(struct futhark_cpu_context_config *cfg);
void futhark_cpu_context_free(struct futhark_cpu_context *ctx);
int futhark_cpu_context_sync(struct futhark_cpu_context *ctx);
char *futhark_cpu_context_get_error(struct futhark_cpu_context *ctx);

/*
 * Arrays
*/

struct f64_1d ;
struct futhark_cpu_f64_1d *futhark_cpu_new_f64_1d(struct futhark_cpu_context *ctx,
                                          double *data, int dim0);
struct futhark_cpu_f64_1d *futhark_cpu_new_raw_f64_1d(struct futhark_cpu_context *ctx,
                                              char *data, int offset, int dim0);
int futhark_cpu_free_f64_1d(struct futhark_cpu_context *ctx,
                        struct futhark_cpu_f64_1d *arr);
int futhark_cpu_values_f64_1d(struct futhark_cpu_context *ctx,
                          struct futhark_cpu_f64_1d *arr, double *data);
char *futhark_cpu_values_raw_f64_1d(struct futhark_cpu_context *ctx,
                                struct futhark_cpu_f64_1d *arr);
int64_t *futhark_cpu_shape_f64_1d(struct futhark_cpu_context *ctx,
                              struct futhark_cpu_f64_1d *arr);

/*
 * Opaque values
*/


/*
 * Entry points
*/

int futhark_cpu_entry_sum(struct futhark_cpu_context *ctx, double *out0, const
                      struct futhark_cpu_f64_1d *in0);
int futhark_cpu_entry_mean(struct futhark_cpu_context *ctx, double *out0, const
                       struct futhark_cpu_f64_1d *in0);
int futhark_cpu_entry_variance(struct futhark_cpu_context *ctx, double *out0, const
                           struct futhark_cpu_f64_1d *in0);
int futhark_cpu_entry_skew(struct futhark_cpu_context *ctx, double *out0, const
                       struct futhark_cpu_f64_1d *in0);
int futhark_cpu_entry_kurtosis(struct futhark_cpu_context *ctx, double *out0, const
                           struct futhark_cpu_f64_1d *in0);
int futhark_cpu_entry_stddev(struct futhark_cpu_context *ctx, double *out0, const
                         struct futhark_cpu_f64_1d *in0);

/*
 * Miscellaneous
*/

void futhark_cpu_debugging_report(struct futhark_cpu_context *ctx);
