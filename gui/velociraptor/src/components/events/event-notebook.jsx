import React from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import NotebookRenderer from '../notebooks/notebook-renderer.jsx';
import _ from 'lodash';
import UserConfig from '../core/user.jsx';
import moment from 'moment';

const POLL_TIME = 5000;

export const get_notebook_id = (artifact, client_id)=>{
    return "N.E." + artifact + "-" + client_id;
};


export default class EventNotebook extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        artifact: PropTypes.string,
        client_id: PropTypes.string,
        start_time: PropTypes.number,
        end_time: PropTypes.number,
    };

    state = {
        notebook: {},
        notebook_id: "",
        loading: true,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let current_notebook_id = get_notebook_id(
            this.props.artifact, this.props.client_id);
        let prev_notebook_id = get_notebook_id(
            prevProps.artifact, prevProps.client_id);
        if (prev_notebook_id !== current_notebook_id) {
            // Re-render the table if the hunt id changes.
            this.fetchNotebooks();
        };
        return false;
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }


    formatTime = ts=>{
        let timezone = this.context.traits.timezone || "UTC";
        return moment.tz(ts, timezone).format();
    }

    fetchNotebooks = () => {
        if (!this.props.artifact || !this.props.client_id) {
            return;
        }

        let notebook_id = get_notebook_id(this.props.artifact,
                                          this.props.client_id);
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
                name: "Notebook for Event Artifact " + this.props.artifact,
                notebook_id: notebook_id,
                context: {
                    type: "event",
                    event_artifact: this.props.artifact,
                    client_id: this.props.client_id,
                    start_time: parseInt(this.props.start_time || 0),
                },
                // Hunt notebooks are all public.
                public: true,
                env: [
                    {key: "ArtifactName", value: this.props.artifact},
                    {key: "ClientId", value: this.props.client_id},
                    {key: "StartTime",
                     value: this.formatTime(this.props.start_time)},
                    {key: "EndTime",
                     value: this.formatTime(this.props.end_time)},
                    {key: "NotebookId", value: notebook_id},
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
        return (
            <NotebookRenderer
              notebook={this.state.notebook}
              fetchNotebooks={this.fetchNotebooks}
            />
        );
    }
};
