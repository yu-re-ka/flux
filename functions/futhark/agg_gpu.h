/*
 * Headers
*/

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#define CL_USE_DEPRECATED_OPENCL_1_2_APIS
#ifdef __APPLE__
#include <OpenCL/cl.h>
#else
#include <CL/cl.h>
#endif


/*
 * Initialisation
*/

int futhark_gpu_get_num_sizes(void);
const char *futhark_gpu_get_size_name(int);
const char *futhark_gpu_get_size_class(int);
const char *futhark_gpu_get_size_entry(int);
struct futhark_gpu_context_config ;
struct futhark_gpu_context_config *futhark_gpu_context_config_new(void);
void futhark_gpu_context_config_free(struct futhark_gpu_context_config *cfg);
void futhark_gpu_context_config_set_debugging(struct futhark_gpu_context_config *cfg,
                                          int flag);
void futhark_gpu_context_config_set_logging(struct futhark_gpu_context_config *cfg,
                                        int flag);
void futhark_gpu_context_config_set_device(struct futhark_gpu_context_config *cfg, const
                                       char *s);
void futhark_gpu_context_config_set_platform(struct futhark_gpu_context_config *cfg,
                                         const char *s);
void futhark_gpu_context_config_dump_program_to(struct futhark_gpu_context_config *cfg,
                                            const char *path);
void
futhark_gpu_context_config_load_program_from(struct futhark_gpu_context_config *cfg,
                                         const char *path);
void
futhark_gpu_context_config_set_default_group_size(struct futhark_gpu_context_config *cfg,
                                              int size);
void
futhark_gpu_context_config_set_default_num_groups(struct futhark_gpu_context_config *cfg,
                                              int num);
void
futhark_gpu_context_config_set_default_tile_size(struct futhark_gpu_context_config *cfg,
                                             int num);
void
futhark_gpu_context_config_set_default_threshold(struct futhark_gpu_context_config *cfg,
                                             int num);
int futhark_gpu_context_config_set_size(struct futhark_gpu_context_config *cfg, const
                                    char *size_name, size_t size_value);
struct futhark_gpu_context ;
struct futhark_gpu_context *futhark_gpu_context_new(struct futhark_gpu_context_config *cfg);
struct futhark_gpu_context
*futhark_gpu_context_new_with_command_queue(struct futhark_gpu_context_config *cfg,
                                        cl_command_queue queue);
void futhark_gpu_context_free(struct futhark_gpu_context *ctx);
int futhark_gpu_context_sync(struct futhark_gpu_context *ctx);
char *futhark_gpu_context_get_error(struct futhark_gpu_context *ctx);
int futhark_gpu_context_clear_caches(struct futhark_gpu_context *ctx);
cl_command_queue futhark_gpu_context_get_command_queue(struct futhark_gpu_context *ctx);

/*
 * Arrays
*/

struct f64_1d ;
struct futhark_gpu_f64_1d *futhark_gpu_new_f64_1d(struct futhark_gpu_context *ctx,
                                          double *data, int dim0);
struct futhark_gpu_f64_1d *futhark_gpu_new_raw_f64_1d(struct futhark_gpu_context *ctx,
                                              cl_mem data, int offset,
                                              int dim0);
int futhark_gpu_free_f64_1d(struct futhark_gpu_context *ctx,
                        struct futhark_gpu_f64_1d *arr);
int futhark_gpu_values_f64_1d(struct futhark_gpu_context *ctx,
                          struct futhark_gpu_f64_1d *arr, double *data);
cl_mem futhark_gpu_values_raw_f64_1d(struct futhark_gpu_context *ctx,
                                 struct futhark_gpu_f64_1d *arr);
int64_t *futhark_gpu_shape_f64_1d(struct futhark_gpu_context *ctx,
                              struct futhark_gpu_f64_1d *arr);

/*
 * Opaque values
*/


/*
 * Entry points
*/

int futhark_gpu_entry_sum(struct futhark_gpu_context *ctx, double *out0, const
                      struct futhark_gpu_f64_1d *in0);
int futhark_gpu_entry_mean(struct futhark_gpu_context *ctx, double *out0, const
                       struct futhark_gpu_f64_1d *in0);
int futhark_gpu_entry_variance(struct futhark_gpu_context *ctx, double *out0, const
                           struct futhark_gpu_f64_1d *in0);
int futhark_gpu_entry_skew(struct futhark_gpu_context *ctx, double *out0, const
                       struct futhark_gpu_f64_1d *in0);
int futhark_gpu_entry_kurtosis(struct futhark_gpu_context *ctx, double *out0, const
                           struct futhark_gpu_f64_1d *in0);
int futhark_gpu_entry_stddev(struct futhark_gpu_context *ctx, double *out0, const
                         struct futhark_gpu_f64_1d *in0);

/*
 * Miscellaneous
*/

void futhark_gpu_debugging_report(struct futhark_gpu_context *ctx);
