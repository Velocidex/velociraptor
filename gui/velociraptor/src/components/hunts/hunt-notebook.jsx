import React from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import NotebookRenderer from '../notebooks/notebook-renderer.jsx';
import _ from 'lodash';
import T from '../i8n/i8n.jsx';

const POLL_TIME = 5000;


export default class HuntNotebook extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    state = {
        notebook: {},
        loading: true,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let prev_hunt_id = prevProps.hunt && prevProps.hunt.hunt_id;
        let current_hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (prev_hunt_id !== current_hunt_id) {
            // Re-render the table if the hunt id changes.
            this.fetchNotebooks();
        };
        return false;
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    fetchNotebooks = () => {
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (!hunt_id) {
            return;
        }

        let notebook_id = "N." + hunt_id;
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

            let request = {
                name: T("Notebook for Hunt", hunt_id),
                description: this.props.hunt.description ||
                    T("This is a notebook for processing a hunt."),
                notebook_id: notebook_id,
                context: {
                    type: "Hunt",
                    hunt_id: hunt_id,
                },
                // Hunt notebooks are all public.
                public: true,
                env: [
                    {key: "HuntId", value: hunt_id},
                ],
            };

            api.post('v1/NewNotebook', request, this.source.token).then((response) => {
                if (response.cancel) return;
                let cell_metadata = response.data && response.data.cell_metadata;
                if (_.isEmpty(cell_metadata)) {
                    return;
                }

                this.fetchNotebooks();
            });
        });
    }

    render() {
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;

        return (
            <> {!_.isEmpty(this.state.notebook) &&
                <NotebookRenderer
                  env={{
                      hunt_id: hunt_id,
                  }}
                  notebook={this.state.notebook}
                  fetchNotebooks={this.fetchNotebooks}
                />
               }
            </>
        );
    }
};
