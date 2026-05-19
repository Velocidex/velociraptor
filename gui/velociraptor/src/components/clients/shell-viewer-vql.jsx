

class _VeloVQLCell extends Component {
    static propTypes = {
        flow: PropTypes.object,
        client: PropTypes.object,
        fetchLastShellCollections: PropTypes.func.isRequired,

        // React router props.
        history: PropTypes.object,
    }

    state = {
        loaded: false,
        showDeleteWizard: false,
    }

    getInput = () => {
        if (!this.props.flow || !this.props.flow.request) {
            return "";
        }

        // Figure out the command we requested.
        var parameters = requestToParameters(this.props.flow.request);
        for (let k in parameters) {
            let params = parameters[k];
            let command = params.Command;
            if(_.isString(command)){
                return command;
            }
        }
        return "";
    }

    viewFlow = target=>{
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        let client_id = this.props.flow.client_id;
        let session_id = this.props.flow.session_id;
        this.props.history.push('/collected/' + client_id +
                                "/" + session_id + "/" + target);
    }

    cancelFlow = (e) => {
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        api.post('v1/CancelFlow', {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
        }, this.source.token).then(function() {
            this.props.fetchLastShellCollections();
        }.bind(this));
    };

    aceConfig = (ace) => {
        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25,
            placeholder: T("Type VQL to run on the client"),
            readOnly: true,
        });

        this.setState({ace: ace});
    };

    render() {
        let buttons = [];

        // The cell can be collapsed ( inside an inset well) or
        // expanded. These buttons can switch between the two modes.
        if (this.state.collapsed) {
            buttons.push(
                <ToolTip tooltip={T("Expand")} key={getKey()} >
                  <Button variant="default"
                          onClick={() => this.setState({collapsed: false})} >
                    <i><FontAwesomeIcon icon="expand"/></i>
                  </Button>
                </ToolTip>
            );
        } else {
            buttons.push(
                <ToolTip tooltip={T("Collapse")} key={getKey()} >
                  <Button variant="default"
                          onClick={() => this.setState({collapsed: true})} >
                    <i><FontAwesomeIcon icon="compress"/></i>
                  </Button>
                </ToolTip>
            );
        }

        // Button to load the output from the server (it could be
        // large so we don't fetch it until the user asks)
        if (this.state.loaded) {
            buttons.push(
                <ToolTip tooltip={T("Hide Output")} key={getKey()} >
                  <Button variant="default"
                          onClick={() => this.setState({"loaded": false})} >
                    <i><FontAwesomeIcon icon="eye-slash"/></i>
                  </Button>
                </ToolTip>
            );
        } else {
            buttons.push(
                <ToolTip tooltip={T("Load Output")} key={getKey()} >
                  <Button variant="default"
                          onClick={() => this.setState({"loaded": true})}>
                    <i><FontAwesomeIcon icon="eye"/></i>
                  </Button>
                </ToolTip>
            );
        }

        let flow_status = [
            <Button variant="outline-info" key={getKey()}
                    onClick={e=>{this.viewFlow("overview");}}
              >
              <i><FontAwesomeIcon icon="external-link-alt"/></i>
            </Button>];

        // If the flow is currently running we may be able to stop it.
        if (this.props.flow.state  === 'RUNNING') {
            buttons.push(
                <ToolTip tooltip={T("Stop")}  key={getKey()} >
                <Button variant="default"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </Button>
                </ToolTip>
            );

            flow_status.push(
                <Button variant="outline-info" key={getKey()} disabled>
                  <i><FontAwesomeIcon icon="spinner" spin /></i>
                  <VeloTimestamp usec={this.props.flow.create_time/1000} />
                by {this.props.flow.request.creator}
                </Button>
            );

        } else if (this.props.flow.state  === 'FINISHED') {
            flow_status.push(
                <Button variant="outline-info" key={getKey()} disabled>
                  <VeloTimestamp usec={this.props.flow.active_time/1000} />
                  by {this.props.flow.request.creator}
                </Button>
            );

        } else if (this.props.flow.state  === 'ERROR') {
            flow_status.push(
                <Button variant="outline-info" key={getKey()} disabled>
                  <i><FontAwesomeIcon icon="exclamation"/></i>
                <VeloTimestamp usec={this.props.flow.create_time/1000} />
            by {this.props.flow.request.creator}
                </Button>
            );
        }

        flow_status.push(
            <ToolTip tooltip={T("Delete")} key={getKey()}>
              <Button variant="default"
                      onClick={()=>this.setState({showDeleteWizard: true})}>
                <i><FontAwesomeIcon icon="trash"/></i>
              </Button>
            </ToolTip>
        );

        let output = <div></div>;
        if (this.state.loaded) {
            let artifact = this.props.flow && this.props.flow.request &&
                this.props.flow.request.artifacts && this.props.flow.request.artifacts[0];
            let params = {
                artifact: artifact,
                client_id: this.props.flow.client_id,
                flow_id: this.props.flow.session_id,
            };
            output = [<VeloPagedTable params={params} key={getKey()} />];

            if (this.props.flow.state  === 'ERROR') {
                output.push(<Button variant="danger" key="ERROR"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" >
                              {T('Error')}
                            </Button>);
            } else {
                output.push(<Button variant="link" key="Logs"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" >
                              {T('Logs')}
                            </Button>);
            }
        }

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flows={[this.props.flow]}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                  }}
                />
              }
              <div className={classNames({
                       collapsed: this.state.collapsed,
                       expanded: !this.state.collapsed,
                       'shell-cell': true,
                   })}>

                <div className='notebook-input'>
                  <div className="cell-toolbar">
                    <div className="btn-group" role="group">
                      { buttons }
                    </div>
                    <div className="btn-group float-right" role="group">
                      { flow_status }
                    </div>
                  </div>

                  <VeloAce text={this.getInput()} mode="sql"
                           aceConfig={this.aceConfig}
                  />
                </div>
                {output}
              </div>
            </>
        );
    };
};

const VeloVQLCell = withRouter(_VeloVQLCell);
