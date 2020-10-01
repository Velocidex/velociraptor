import React from 'react';
import PropTypes from 'prop-types';

import VeloTable, { PrepareData } from '../core/table.js';

import _ from 'lodash';

import api from '../core/api-service.js';

export default class NotebookTableRenderer extends React.Component {
    static propTypes = {
        params: PropTypes.object,
    };

    state = {
        pageData: {},
        params: {},
        loading: false,
    }

    componentDidMount() {
        this.getData();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(this.props.params, this.state.params)) {
            this.setState({params: this.props.params});
            this.getData();
        }
    }

    getData = () => {
        this.setState({loading: true});
        api.get("api/v1/GetTable", this.props.params).then((response) => {
            this.setState({loading: false, pageData: PrepareData(response.data)});
        });
    }

    render() {
        let body = <div>No data available</div>;

        if (this.state.pageData && this.state.pageData.columns) {
            body = <VeloTable
                     className="col-12"
                     rows={this.state.pageData.rows}
                     columns={this.state.pageData.columns} />;
        }

        return (
            <>
              {body}
            </>
        );
    }
};
