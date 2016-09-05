this_dir = File.expand_path(File.dirname(__FILE__))
lib_dir = File.join(this_dir, 'lib')
$LOAD_PATH.unshift(lib_dir) unless $LOAD_PATH.include?(lib_dir)

require 'google/protobuf'

require File.join(this_dir, 'coprocess_mini_request_object_pb')
require File.join(this_dir, 'coprocess_session_state' )
require File.join(this_dir, 'coprocess_common' )

Google::Protobuf::DescriptorPool.generated_pool.build do
  add_message "coprocess.Object" do
    optional :hook_type, :enum, 1, "coprocess.HookType"
    optional :hook_name, :string, 2
    optional :request, :message, 3, "coprocess.MiniRequestObject"
    optional :session, :message, 4, "coprocess.SessionState"
    map :metadata, :string, :string, 5
    map :spec, :string, :string, 6
  end
end

module Coprocess
  Object = Google::Protobuf::DescriptorPool.generated_pool.lookup("coprocess.Object").msgclass
end