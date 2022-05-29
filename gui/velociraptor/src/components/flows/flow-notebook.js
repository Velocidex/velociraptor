import React from 'react';
import PropTypes from 'prop-types';
import axios from 'axios';
import api from '../core/api-service.js';
import NotebookRenderer from '../notebooks/notebook-renderer.js';
import _ from 'lodash';
import T from '../i8n/i8n.js';
const POLL_TIME = 5000;


export default class FlowNotebook extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        notebook: {},
        loading: true,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        let current_flow_id = this.props.flow && this.props.flow.session_id;
        if (prev_flow_id !== current_flow_id) {
            this.fetchNotebooks();
        };
        return false;
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    getCellVQL = (flow) => {
        var query = "SELECT * \nFROM source(\n";
        var sources = flow["artifacts_with_results"] || flow["request"]["artifacts"];
        for (var i=1; i<sources.length; i++) {
            query += "    -- artifact='" + sources[i] + "',\n";
        }
        query += "    artifact='" + sources[0] + "'\n) LIMIT 50";

        return query;
    }

    fetchNotebooks = () => {
        let client_id = this.props.flow && this.props.flow.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;
        if (!flow_id || !client_id) {
            return;
        }

        let notebook_id = "N." + flow_id + "-" + client_id;

        this.source.cancel();
        this.source = axios.CancelToken.source();

        this.setState({loading: true});
        api.get("v1/GetNotebooks", {
            notebook_id: notebook_id,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];

            if (notebooks.length > 0) {
                this.setState({notebook: notebooks[0], loading: false});
                return;
            }

            // If no notebook was found, try to create one.
            let request = {
                name: T("Notebook for Collection", flow_id),
                notebook_id: notebook_id,
                context: {
                    type: "flow",
                    flow_id: flow_id,
                    client_id: client_id,
                },
                // Flow notebooks are all public.
                public: true,
                env: [
                    {key: "FlowId", value: flow_id},
                    {key: "ClientId", value: client_id},
                    {key: "NotebookId", value: notebook_id},
                ],
            };

            api.post('v1/NewNotebook', request,
                     this.source.token).then((response) => {
                let cell_metadata = response.data && response.data.cell_metadata;
                if (_.isEmpty(cell_metadata)) {
                    return;
                }

                this.fetchNotebooks();
            });
        });
    }

    render() {
        return (
            <> {!_.isEmpty(this.state.notebook) &&
                <NotebookRenderer
                  notebook={this.state.notebook}
                  fetchNotebooks={this.fetchNotebooks}
                />
               }
            </>
        );
    }
};
